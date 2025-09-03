/*
 * Index page
 *
 * Copyright (C) 2024  Runxi Yu <https://runxiyu.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func handleIndex(w http.ResponseWriter, req *http.Request) (string, int, error) {
	userID, username, department, email, _, err := getUserInfoFromRequest(req)
	if errors.Is(err, errNoCookie) || errors.Is(err, errNoSuchUser) {
		authURL, err2 := generateAuthorizationURL()
		if err2 != nil {
			return "", -1, err2
		}
		var noteString string
		if errors.Is(err, errNoSuchUser) {
			noteString = "Your browser provided an invalid session cookie."
		}
		err2 = tmpl.ExecuteTemplate(
			w,
			"login",
			struct {
				AuthURL string
				Notes   string
			}{
				authURL,
				noteString,
			},
		)
		if err2 != nil {
			return "", -1, wrapError(errCannotWriteTemplate, err2)
		}
		return "", -1, nil
	} else if err != nil {
		return "", -1, err
	}

	/* TODO: The below should be completed on-update. */
	type groupT struct {
		Handle  string
		Name    string
		Courses *map[int]*courseT
	}
	_groups := make(map[string]groupT)
	for k, v := range courseGroups {
		_coursemap := make(map[int]*courseT)
		_groups[k] = groupT{
			Handle:  k,
			Name:    v,
			Courses: &_coursemap,
		}
	}
	err = nil
	courses.Range(func(key, value interface{}) bool {
		courseID, ok := key.(int)
		if !ok {
			err = errType
			return false
		}
		course, ok := value.(*courseT)
		if !ok {
			err = errType
			return false
		}
		if department != staffDepartment {
			if yearGroupsNumberBits[department]&course.YearGroups == 0 {
				return true
			}
		}
		(*_groups[course.Group].Courses)[courseID] = course
		return true
	})
	if err != nil {
		return "", -1, err
	}

	if department == staffDepartment {
		StatesDereferenced := map[string]struct {
			S     uint32
			Sched *string
		}{}
		for k, v := range states {
			scheduleTime := schedules[k].Load()
			var scheduleString *string
			if scheduleTime != nil {
				_1 := scheduleTime.Format("2006-01-02T15:04")
				scheduleString = &_1
			}
			StatesDereferenced[k] = struct {
				S     uint32
				Sched *string
			}{
				S:     atomic.LoadUint32(v),
				Sched: scheduleString,
			}
		}

		studentishes, err := getStudentsThatHaveNotConfirmedTheirChoicesYetIncludingThoseWhoHaveNotLoggedInAtAll(req.Context())
		if err != nil {
			return "", -1, err
		}

		ee := []string{}
		for _, v := range studentishes {
			ee = append(ee, v.Email)
		}

		err = tmpl.ExecuteTemplate(
			w,
			"staff",
			struct {
				Name   string
				States map[string]struct {
					S     uint32
					Sched *string
				}
				StatesOr uint32
				Groups   *map[string]groupT
				Students []studentish
				Ee       []string
			}{
				username,
				StatesDereferenced,
				func() uint32 {
					var ret uint32 /* all zero bits */
					for _, v := range StatesDereferenced {
						ret |= v.S
					}
					return ret
				}(),
				&_groups,
				studentishes,
				ee,
			},
		)
		if err != nil {
			return "", -1, wrapError(errCannotWriteTemplate, err)
		}
		return "", -1, nil
	}

	_state, ok := states[department]
	if !ok {
		return "", -1, errNoSuchYearGroup
	}
	if atomic.LoadUint32(_state) == 0 {
		err := tmpl.ExecuteTemplate(
			w,
			"student_disabled",
			struct {
				Name       string
				Department string
			}{
				username,
				department,
			},
		)
		if err != nil {
			return "", -1, wrapError(errCannotWriteTemplate, err)
		}
		return "", -1, nil
	}
	sportRequired, err := getCourseTypeMinimumForYearGroup(
		department, sport,
	)
	if err != nil {
		return "", -1, err
	}
	nonSportRequired, err := getCourseTypeMinimumForYearGroup(
		department, nonSport,
	)
	if err != nil {
		return "", -1, err
	}

	// get the student id 12345 from email s12345@domain
	userpart, _, found := strings.Cut(email, "@")
	if !found {
		return "", http.StatusInternalServerError, fmt.Errorf("email %q does not contain @", email)
	}
	if len(userpart) < 2 || (userpart[0] != 's' && userpart[0] != 'S') {
		return "", http.StatusInternalServerError, fmt.Errorf("email %q does not start with s or S", email)
	}
	studentID := userpart[1:]

	// TODO
	rows, err := db.Query(req.Context(), `SELECT course_id FROM pre_selected WHERE student_id = $1`, studentID)
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("failed to fetch pre_selected choices: %w", err)
	}
	defer rows.Close()

	now := time.Now()

	for rows.Next() {
		var courseID int
		if err := rows.Scan(&courseID); err != nil {
			return "", http.StatusInternalServerError, fmt.Errorf("failed to scan course_id: %w", err)
		}

		_, err = db.Exec(req.Context(),
			`INSERT INTO choices (userid, courseid, seltime, forced) VALUES ($1, $2, $3, true)`,
			userID, courseID, now.UnixMicro())
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgErrUniqueViolation {
				continue
			}
			return "", http.StatusInternalServerError, fmt.Errorf("failed to insert choice for course %d: %w", courseID, err)
		}

		_course, ok := courses.Load(courseID)
		if !ok {
			return "", -1, errNoSuchCourse
		}
		course, ok := _course.(*courseT)
		if !ok {
			return "", -1, errType
		}
		if course == nil {
			return "", -1, errNoSuchCourse
		}

		func() {
			course.SelectedLock.Lock()
			defer course.SelectedLock.Unlock()
			atomic.AddUint32(&course.Selected, 1)
		}()
	}

	if err := rows.Err(); err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("error iterating over pre_selected rows: %w", err)
	}

	err = tmpl.ExecuteTemplate(
		w,
		"student",
		struct {
			Name       string
			Department string
			Groups     *map[string]groupT
			Required   struct {
				Sport    int
				NonSport int
			}
		}{
			username,
			department,
			&_groups,
			struct {
				Sport    int
				NonSport int
			}{sportRequired, nonSportRequired},
		},
	)
	if err != nil {
		return "", -1, wrapError(errCannotWriteTemplate, err)
	}
	return "", -1, nil
}

type studentish struct {
	Name       string
	Email      string
	Department string
	Status     string
}
