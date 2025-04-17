package main

import (
	"context"
	"strconv"
	"strings"
)

func getStudentsThatHaveNotConfirmedTheirChoicesYetIncludingThoseWhoHaveNotLoggedInAtAll(ctx context.Context) (res []studentish, err error) {
	ni, err := queryNameID(ctx, "SELECT name, id FROM expected_students ORDER BY id")
	if err != nil {
		return nil, wrapError(errUnexpectedDBError, err)
	}

	rows, err := db.Query(
		ctx,
		"SELECT name, email, department, confirmed FROM users ORDER BY email",
	)
	if err != nil {
		return nil, wrapError(errUnexpectedDBError, err)
	}
	for {
		if !rows.Next() {
			err := rows.Err()
			if err != nil {
				return nil, wrapError(errUnexpectedDBError, err)
			}
			break
		}
		var currentUserName, currentEmail, currentDepartment string
		var currentConfirmed bool
		err := rows.Scan(
			&currentUserName,
			&currentEmail,
			&currentDepartment,
			&currentConfirmed,
		)
		if err != nil {
			return nil, wrapError(errUnexpectedDBError, err)
		}
		if currentDepartment == staffDepartment {
			continue
		}
		unamepart, _, _ := strings.Cut(currentEmail, "@")
		unamepart = strings.TrimPrefix(strings.TrimPrefix(unamepart, "s"), "S")
		nii, _ := strconv.ParseInt(unamepart, 10, 64)
		delete(ni, nii)

		if currentConfirmed {
			continue
		}

		res = append(
			res,
			studentish{
				Name:       currentUserName,
				Email:      currentEmail,
				Department: currentDepartment,
				Status:     "Hasn’t confirmed yet",
			},
		)
	}

	for k, v := range ni {
		res = append(
			res,
			studentish{
				Name:       v,
				Email:      "s" + strconv.FormatInt(k, 10) + "@ykpaoschool.cn",
				Department: "Unknown",
				Status:     "Never logged in",
			},
		)
	}

	return
}
