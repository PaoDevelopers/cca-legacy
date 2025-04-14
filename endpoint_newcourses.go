/*
 * Overwrite courses with uploaded CSV
 *
 * Copyright (C) 2024  Runxi Yu <https://runxiyu.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

func handleNewCourses(w http.ResponseWriter, req *http.Request) (string, int, error) {
	if req.Method != http.MethodPost {
		return "", http.StatusMethodNotAllowed, errPostOnly
	}

	_, _, department, _, err := getUserInfoFromRequest(req)
	if err != nil {
		return "", -1, err
	}
	if department != staffDepartment {
		return "", http.StatusForbidden, errStaffOnly
	}

	if !func() bool {
		for _, v := range states {
			if atomic.LoadUint32(v) != 0 {
				return false
			}
		}
		return true
	}() {
		return "", http.StatusBadRequest, errDisableStudentAccessFirst
	}

	/* TODO: Race condition. The global state may need to be write-locked. */

	file, fileHeader, err := req.FormFile("coursecsv")
	if err != nil {
		return "", http.StatusBadRequest, wrapError(errFormNoFile, err)
	}

	if fileHeader.Header.Get("Content-Type") != "text/csv" {
		return "", http.StatusBadRequest, errNotACSV
	}

	csvReader := csv.NewReader(file)
	titleLine, err := csvReader.Read()
	if err != nil {
		return "", http.StatusBadRequest, wrapError(errCannotReadCSV, err)
	}
	if titleLine == nil {
		return "", -1, errUnexpectedNilCSVLine
	}
	if len(titleLine) != 10 {
		return "", -1, wrapAny(
			errBadCSVFormat,
			"expecting 10 fields on the first line",
		)
	}
	var titleIndex, maxIndex, teacherIndex, locationIndex,
		typeIndex, groupIndex, sectionIDIndex,
		courseIDIndex, yearGroupsIndex, legalSexIndex int = -1, -1, -1, -1, -1, -1, -1, -1, -1, -1
	for i, v := range titleLine {
		switch v {
		case "Title":
			titleIndex = i
		case "Max":
			maxIndex = i
		case "Teacher":
			teacherIndex = i
		case "Location":
			locationIndex = i
		case "Type":
			typeIndex = i
		case "Group":
			groupIndex = i
		case "Section ID":
			sectionIDIndex = i
		case "Course ID":
			courseIDIndex = i
		case "Year Groups":
			yearGroupsIndex = i
		case "Legal Sex Requirements":
			legalSexIndex = i
		default:
			return "", http.StatusBadRequest, wrapAny(
				errBadCSVFormat,
				fmt.Sprintf(
					"unexpected field \"%s\" on the first line",
					v,
				),
			)
		}
	}

	if titleIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Title",
		)
	}
	if maxIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Max",
		)
	}
	if teacherIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Teacher",
		)
	}
	if locationIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Location",
		)
	}
	if typeIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Type",
		)
	}
	if groupIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Group",
		)
	}
	if courseIDIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Course ID",
		)
	}
	if sectionIDIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Section ID",
		)
	}
	if yearGroupsIndex == -1 {
		return "", http.StatusBadRequest, wrapAny(
			errMissingCSVColumn,
			"Year Groups",
		)
	}

	lineNumber := 1
	ok, statusCode, err := func(ctx context.Context) (
		retBool bool,
		retStatus int,
		retErr error,
	) {
		tx, err := db.Begin(ctx)
		if err != nil {
			return false, -1, wrapError(errUnexpectedDBError, err)
		}
		defer func() {
			err := tx.Rollback(ctx)
			if err != nil && (!errors.Is(err, pgx.ErrTxClosed)) {
				retBool, retStatus, retErr = false, -1, wrapError(
					errUnexpectedDBError,
					err,
				)
				return
			}
		}()
		_, err = tx.Exec(
			ctx,
			"DELETE FROM choices",
		)
		if err != nil {
			return false, -1, wrapError(errUnexpectedDBError, err)
		}
		_, err = tx.Exec(
			ctx,
			"UPDATE users SET confirmed = false",
		)
		if err != nil {
			return false, -1, wrapError(errUnexpectedDBError, err)
		}
		_, err = tx.Exec(
			ctx,
			"DELETE FROM courses",
		)
		if err != nil {
			return false, -1, wrapError(errUnexpectedDBError, err)
		}

		for {
			lineNumber++
			line, err := csvReader.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return false, -1, wrapError(
					errCannotReadCSV,
					err,
				)
			}
			if line == nil {
				return false, -1, wrapError(
					errCannotReadCSV,
					errUnexpectedNilCSVLine,
				)
			}
			if len(line) != 10 {
				return false, -1, wrapAny(
					errInsufficientFields,
					fmt.Sprintf(
						"line %d has a wrong number of items",
						lineNumber,
					),
				)
			}
			if !checkCourseType(line[typeIndex]) {
				return false, -1, wrapAny(errInvalidCourseType,
					fmt.Sprintf(
						"line %d has invalid course type \"%s\"\nallowed course types: %s",
						lineNumber,
						line[typeIndex],
						strings.Join(
							getKeysOfMap(courseTypes),
							", ",
						),
					),
				)
			}
			if !checkCourseGroup(line[groupIndex]) {
				return false, -1, wrapAny(errInvalidCourseGroup,
					fmt.Sprintf(
						"line %d has invalid course group \"%s\"\nallowed course groups: %s",
						lineNumber,
						line[groupIndex],
						strings.Join(
							getKeysOfMap(courseGroups),
							", ",
						),
					),
				)
			}
			yearGroupsSpec, err := yearGroupsStringToNumber(line[yearGroupsIndex])
			if err != nil {
				return false, -1, err
			}

			_, err = tx.Exec(
				ctx,
				"INSERT INTO courses(nmax, title, teacher, location, ctype, cgroup, section_id, course_id, legal_sex_requirements, year_groups, forced) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, false)",
				line[maxIndex],
				line[titleIndex],
				line[teacherIndex],
				line[locationIndex],
				line[typeIndex],
				line[groupIndex],
				line[sectionIDIndex],
				line[courseIDIndex],
				line[legalSexIndex],
				yearGroupsSpec,
			)
			if err != nil {
				return false, -1, wrapError(
					errUnexpectedDBError,
					err,
				)
			}
		}
		err = tx.Commit(ctx)
		if err != nil {
			return false, -1, wrapError(errUnexpectedDBError, err)
		}
		return true, -1, nil
	}(req.Context())
	if !ok {
		return "", statusCode, err
	}

	courses.Range(func(key, _ interface{}) bool {
		courses.Delete(key)
		return true
	})
	err = setupCourses(req.Context())
	if err != nil {
		return "", -1, wrapError(errWhileSetttingUpCourseTablesAgain, err)
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return "", -1, nil
}
