/*
 * Custom OAUTH 2.0 implementation for the CCA Selection Service
 *
 * Copyright (C) 2024  Runxi Yu <https://runxiyu.org>
 * SPDX-License-Identifier: AGPL-3.0-or-later
 */

package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var myKeyfunc keyfunc.Keyfunc

const tokenLength = 20

/*
 * These are the claims in the JSON Web Token received from the client, after
 * it redirects from the authorize endpoint. Some of these fields must be
 * explicitly selected in the Azure app registration and might appear as
 * zero strings if it hasn't been configured correctly.
 */
type MicrosoftAuthClaims struct {
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Oid    string   `json:"oid"`
	Groups []string `json:"groups"`
	jwt.RegisteredClaims
}

func generateAuthorizationURL() (string, error) { // \codelabel{generateAuthorizationURL}
	nonce, err := randomString(tokenLength)
	if err != nil {
		return "", err
	}
	/*
	 * Note that here we use a hybrid authentication flow to obtain an
	 * id token for authentication and an authorization code. The
	 * authorization code may be used like any other; i.e., it may be used
	 * to obtain an access token directly, or the refresh token may be used
	 * to gain persistent access to the upstream API. Sometimes I wish that
	 * the JWT in id token could have more claims. The only reason we
	 * presently use a hybrid flow is to use the authorization code to
	 * obtain an access code to call the user info endpoint to fetch the
	 * user's department information.
	 */
	return fmt.Sprintf(
		"https://login.microsoftonline.com/ddd3d26c-b197-4d00-a32d-1ffd84c0c295/oauth2/authorize?client_id=%s&response_type=id_token%%20code&redirect_uri=%s%%2Fauth&response_mode=form_post&scope=openid+profile+email+User.Read&nonce=%s",
		config.Auth.Client,
		config.URL,
		nonce,
	), nil
}

// Handles redirects to the /auth endpoint from the authorize endpoint.
// Expects JSON Web Keys to be already set up correctly; if myKeyfunc is null,
// a null pointer is dereferenced and the goroutine panics.
func handleAuth(w http.ResponseWriter, req *http.Request) (string, int, error) { // \codelabel{handleAuth}
	slog.Info("handleAuth", "method", req.Method, "url", req.URL.String())

	if req.Method != http.MethodPost {
		return "", http.StatusMethodNotAllowed,
			errors.New("only POST is allowed here")
	}

	err := req.ParseForm()
	if err != nil {
		return "", http.StatusBadRequest,
			fmt.Errorf("malformed form: %w", err)
	}

	returnedError := req.PostFormValue("error")
	if returnedError != "" {
		returnedErrorDescription := req.PostFormValue("error_description")
		return "", http.StatusUnauthorized,
			fmt.Errorf("jwt auth returned error: %v: %v",
				returnedError, returnedErrorDescription)
	}

	idTokenString := req.PostFormValue("id_token")
	if idTokenString == "" {
		return "", http.StatusUnauthorized,
			errors.New("insufficient fields: id_token")
	}

	claimsTemplate := &MicrosoftAuthClaims{} //exhaustruct:ignore
	token, err := jwt.ParseWithClaims(
		idTokenString,
		claimsTemplate,
		myKeyfunc.Keyfunc,
	)
	if err != nil {
		return "", http.StatusBadRequest,
			fmt.Errorf("parse jwt claims: %w", err)
	}

	claims, claimsOk := token.Claims.(*MicrosoftAuthClaims)

	slog.Info("token claims", "claims", claims)

	switch {
	case token.Valid:
		break
	case errors.Is(err, jwt.ErrTokenMalformed):
		return "", http.StatusBadRequest,
			fmt.Errorf("malformed jwt: %w", err)
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return "", http.StatusBadRequest,
			fmt.Errorf("invalid jwt signature: %w", err)
	case errors.Is(err, jwt.ErrTokenExpired) ||
		errors.Is(err, jwt.ErrTokenNotValidYet):
		return "", http.StatusBadRequest,
			fmt.Errorf("invalid jwt timing: %w", err)
	default:
		return "", http.StatusBadRequest,
			fmt.Errorf("invalid jwt: %w", err)
	}

	if !claimsOk { // Should never happen, unless MS breaks their API
		return "", http.StatusBadRequest,
			errors.New("failed to unpack claims")
	}

	// If the user has a department override in the config, use that,
	// otherwise just take it from MS's ID Token groups.
	department, ok := getDepartmentByUserIDOverride(claims.Oid)
	if !ok {
		department, ok = getDepartmentByGroups(claims.Groups)
		if !ok {
			return "", http.StatusBadRequest,
				errors.New("unknown department")
		}
	}

	cookieValue, err := randomString(tokenLength) // TODO: Use Go124 API
	if err != nil {
		return "", -1, err
	}

	now := time.Now()
	expr := now.Add(time.Duration(config.Auth.Expr) * time.Second)
	exprU := expr.Unix()

	cookie := http.Cookie{
		Name:     "session",
		Value:    cookieValue,
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   config.Prod,
		Expires:  expr,
	} //exhaustruct:ignore

	http.SetCookie(w, &cookie)

	claims.Email = strings.ToLower(claims.Email)

	localpart, _, ok := strings.Cut(claims.Email, "@")

	if !ok {
		return "", http.StatusBadRequest, errors.New("your email address seems to be invalid. Please contact s22537@stu.ykpaoschool.cn")
	}

	var legalSex string
	studentID := strings.TrimPrefix(strings.TrimPrefix(localpart, "s"), "S")

	tx, err := db.Begin(req.Context())
	if err != nil {
		return "", -1, fmt.Errorf("begin transaction: %w", err)
	}
	/*
		defer func(ctx context.Context) {
			if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				log.Printf("failed to rollback transaction: %v", err)
			}
		}(req.Context())
	*/

	_ = db.QueryRow( // TODO: No legal sex
		req.Context(),
		"SELECT legal_sex from expected_students WHERE id = $1",
		studentID,
	).Scan(&legalSex)

	if legalSex != "" {
		_, err = db.Exec(
			req.Context(),
			"INSERT INTO users (id, name, email, department, session, expr, confirmed, legal_sex) VALUES ($1, $2, $3, $4, $5, $6, false, $7)",
			claims.Oid,
			claims.Name,
			claims.Email,
			department,
			cookieValue,
			exprU,
			legalSex,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgErrUniqueViolation {
				_, err := db.Exec(
					req.Context(),
					"UPDATE users SET (name, email, department, session, expr, legal_sex) = ($1, $2, $3, $4, $5, $7) WHERE id = $6",
					claims.Name,
					claims.Email,
					department,
					cookieValue,
					exprU,
					claims.Oid,
					legalSex,
				)
				if err != nil {
					return "", -1, fmt.Errorf("update user: %w", err)
				}
			} else {
				return "", -1, fmt.Errorf("insert user: %w", err)
			}
		}
	} else {
		if department != "Staff" {
			slog.Warn("student with unknown legal sex", "studentID", studentID, "oid", claims.Oid, "email", claims.Email, "name", claims.Name)
		}

		_, err = db.Exec(
			req.Context(),
			"INSERT INTO users (id, name, email, department, session, expr, confirmed) VALUES ($1, $2, $3, $4, $5, $6, false)",
			claims.Oid,
			claims.Name,
			claims.Email,
			department,
			cookieValue,
			exprU,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgErrUniqueViolation {
				_, err := db.Exec(
					req.Context(),
					"UPDATE users SET (name, email, department, session, expr) = ($1, $2, $3, $4, $5) WHERE id = $6",
					claims.Name,
					claims.Email,
					department,
					cookieValue,
					exprU,
					claims.Oid,
				)
				if err != nil {
					return "", -1, fmt.Errorf("update user: %w", err)
				}
			} else {
				return "", -1, fmt.Errorf("insert user: %w", err)
			}
		}
	}

	log.Printf("%s (%s, %s) just authenticated", claims.Name, claims.Email, claims.Oid)

	err = tx.Commit(req.Context())
	if err != nil {
		return "", -1, fmt.Errorf("commit transaction: %w", err)
	}

	if department == "Staff" {
		http.Redirect(w, req, "/", http.StatusSeeOther)
		return "", -1, nil
	}

	// TODO: Do this in the root page instead
	rows, err := db.Query(req.Context(), `SELECT course_id FROM pre_selected WHERE student_id = $1`, studentID)
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("failed to fetch pre_selected choices: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var courseID int
		if err := rows.Scan(&courseID); err != nil {
			return "", http.StatusInternalServerError, fmt.Errorf("failed to scan course_id: %w", err)
		}

		_, err = db.Exec(req.Context(),
			`INSERT INTO choices (userid, courseid, seltime, forced) VALUES ($1, $2, $3, true)`,
			claims.Oid, courseID, now.UnixMicro())
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

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return "", -1, nil
}

func setupJwks() error {
	var err error
	myKeyfunc, err = keyfunc.NewDefault([]string{config.Auth.Jwks})
	if err != nil {
		return fmt.Errorf("setup jwks: %w", err)
	}
	return nil
}

func getDepartmentByGroups(groups []string) (string, bool) {
	for _, g := range groups {
		d, ok := config.Auth.Departments[g]
		if ok {
			return d, true
		}
	}
	return "", false
}

func getDepartmentByUserIDOverride(userID string) (string, bool) {
	d, ok := config.Auth.Udepts[userID]
	if ok {
		return d, true
	}
	return "", false
}
