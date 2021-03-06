package trade

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tradedb "github.com/AlexSugak/getsky-trade/db"
	"github.com/AlexSugak/getsky-trade/db/models"
	"github.com/AlexSugak/getsky-trade/src/util/logger"
	"github.com/AlexSugak/getsky-trade/src/util/test"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	validator "gopkg.in/go-playground/validator.v9"
)

type FakeAuthenticator struct {
	mock.Mock
}

func (fa *FakeAuthenticator) VerifyPassword(username string, password string) error {
	if username == "testuser" && password != "wrong_password" {
		return nil
	}

	return errors.New("wrong user or password")
}

func FakeRecaptchaChecker(response string) (bool, error) {
	if response == "pass" {
		return true, nil
	}

	return false, nil
}

func setupAuthenticateHandlerTests() func() {
	execSQL("INSERT INTO `%s`.`Users` (UserName, Email, PasswordHash, TimeOffset, CountryCode, StateCode, City, PostalCode, DistanceUnits, Currency, Status) VALUES ('testuser', 'bob@bob.com', 'foo', 0, 'US', 'CA', 'Los Angeles', '', 'mi', 'USD', 1)", dbName)

	return func() {
		clearTables()
	}
}

func TestAuthenticateHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		contentType    string
		url            string
		body           string
		expectedStatus int
	}{
		{
			name:           "should validate content type",
			method:         "POST",
			contentType:    "application/xml",
			url:            "/api/users/authenticate",
			expectedStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:           "should not accept non json body",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users/authenticate",
			body:           `<foo />`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "should validate user and password",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users/authenticate",
			body:           `{"username": "foo", "password": "bar"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "should return OK",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users/authenticate",
			body:           `{"username": "testuser", "password": "foo"}`,
			expectedStatus: 200,
		},
	}

	teardownTests := setupAuthenticateHandlerTests()
	defer teardownTests()

	for _, tc := range tests {
		name := fmt.Sprintf("test case: AuthenticateHandler %s", tc.name)
		req, err := http.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", tc.contentType)

		a := &FakeAuthenticator{}
		sql := sqlx.NewDb(db, "mysql")
		u := tradedb.NewUsers(sql)

		w := httptest.NewRecorder()
		server := &HTTPServer{authenticator: a, users: u, log: logger.InitLogger()}
		handler := server.setupRouter(test.StubSecure)

		handler.ServeHTTP(w, req)
		require.Equal(t, tc.expectedStatus, w.Code, name)
	}
}

func TestRegisterHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		contentType    string
		url            string
		body           string
		expectedStatus int
		username       string
	}{
		{
			name:           "should validate content type",
			method:         "POST",
			contentType:    "application/xml",
			url:            "/api/users",
			expectedStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:           "should not accept non json body",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users",
			body:           `<foo />`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "should require fields",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "should validate email",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users",
			body:           `{"email":"foo","username":"foo","password":"1","timeOffset":1}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "should return StatusBadRequest if captcha is not valid",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users",
			body:           `{"email":"foo1@bar.baz","username":"foo1","password":"1","timeOffset":1,"recaptcha":"not_pass"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "should return OK and insert user",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/users",
			body:           `{"email":"foo1@bar.baz","username":"foo1","password":"1","timeOffset":1,"recaptcha":"pass"}`,
			expectedStatus: http.StatusOK,
			username:       "foo1",
		},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("test case: RegisterHandler %s", tc.name)
		req, err := http.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", tc.contentType)

		a := &FakeAuthenticator{}
		sql := sqlx.NewDb(db, "mysql")
		u := tradedb.NewUsers(sql)

		w := httptest.NewRecorder()
		server := &HTTPServer{authenticator: a, users: u, checkRecaptcha: FakeRecaptchaChecker, log: logger.InitLogger()}
		server.validate = validator.New()
		handler := server.setupRouter(test.StubSecure)

		handler.ServeHTTP(w, req)
		require.Equal(t, tc.expectedStatus, w.Code, name)

		if tc.username != "" {
			user := &struct {
				UserName string `db:"UserName"`
			}{}
			cmd := fmt.Sprintf("SELECT u.UserName FROM %s.Users u WHERE u.UserName = ?", dbName)
			err := sql.Get(user, cmd, tc.username)
			require.NoError(t, err)
			require.Equal(t, tc.username, user.UserName)
		}
	}
}

func setupUpdateUserTests() func() {
	execSQL("INSERT INTO `%s`.`Users` (UserName, Email, PasswordHash, TimeOffset, CountryCode, StateCode, City, PostalCode, DistanceUnits, Currency, Status) VALUES ('testuser', 'testuser@testuser.com', 'foo', 0, 'US', 'CA', 'Los Angeles', '', 'mi', 'USD', 1)", dbName)
	execSQL("INSERT INTO `%s`.`Users` (UserName, Email, PasswordHash, TimeOffset, CountryCode, StateCode, City, PostalCode, DistanceUnits, Currency, Status) VALUES ('alice', 'alice@alice.com', 'foo', 0, 'US', 'CA', 'Los Angeles', '', 'mi', 'USD', 1)", dbName)

	return func() {
		clearTables()
	}
}

func TestUpdateUserSettings(t *testing.T) {
	tests := []struct {
		name                 string
		method               string
		url                  string
		contentType          string
		body                 string
		expectedStatus       int
		expectedBody         string
		expectedUserSettings models.UserSettings
	}{
		{
			name:           "should validate content type",
			method:         "POST",
			contentType:    "application/xml",
			url:            "/api/me/settings",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Invalid content type, expected application/jsonInvalid json request body: EOF",
		},
		{
			name:           "should return 400 when json body is not valid",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/me/settings",
			body:           "<foo />",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid json request body: invalid character '<' looking for beginning of value",
		},
		{
			name:           "should return 400 when specified email is used by another user",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/me/settings",
			body:           `{"timeOffset":0, "email":"alice@alice.com","countryCode":"GR","city":"Athens","postalCode":"0000","distanceUnits":"Athens","currency":"EUR"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `[{"key":"email","message":"Specified email address is already used by another user"}]`,
		},
		{
			name:           "should return 200 when operation successful",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/me/settings",
			body:           `{"timeOffset":1,"email":"bob2@bob2.com","countryCode":null,"city":"New York","postalCode":"9999","distanceUnits":"Athens","currency":"USD","stateCode":null}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "",
			expectedUserSettings: models.UserSettings{
				UserName:      "testuser",
				Email:         "bob2@bob2.com",
				TimeOffset:    1,
				CountryCode:   models.JSONNullString{},
				City:          "New York",
				PostalCode:    "9999",
				DistanceUnits: "Athens",
				Currency:      "USD",
				StateCode:     models.JSONNullString{},
			},
		},
	}

	teardownTests := setupUpdateUserTests()
	defer teardownTests()

	for _, tc := range tests {
		name := fmt.Sprintf("test case: TestUpdateUserSettings %s", tc.name)
		req, err := http.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.contentType)

		require.NoError(t, err)

		sql := sqlx.NewDb(db, "mysql")
		u := tradedb.NewUsers(sql)

		w := httptest.NewRecorder()
		server := &HTTPServer{users: u, log: logger.InitLogger()}
		server.validate = validator.New()

		stubAuthHeader := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("name", "testuser")
				h.ServeHTTP(w, r)
			})
		}

		handler := server.setupRouter(stubAuthHeader)
		handler.ServeHTTP(w, req)

		require.Equal(t, tc.expectedStatus, w.Code, name)
		require.Equal(t, tc.expectedBody, strings.TrimSuffix(w.Body.String(), "\n"), name)

		if tc.expectedUserSettings != (models.UserSettings{}) {
			userSettingds := &models.UserSettings{}

			cmd := fmt.Sprintf("SELECT u.UserName, u.Email, u.TimeOffset, u.CountryCode, u.StateCode, u.City, u.PostalCode, u.DistanceUnits, u.Currency FROM %s.Users u WHERE u.UserName = ?", dbName)
			err := sql.Get(userSettingds, cmd, "testuser")
			require.NoError(t, err)
			require.Equal(t, tc.expectedUserSettings, *userSettingds)
		}
	}
}

func setupChangeUserPasswordTests() func() {
	execSQL("INSERT INTO `%s`.`Users` (UserName, Email, PasswordHash, TimeOffset, CountryCode, StateCode, City, PostalCode, DistanceUnits, Currency, Status) VALUES ('testuser', 'bob@bob.com', 'foo', 0, 'US', 'CA', 'Los Angeles', '', 'mi', 'USD', 1)", dbName)

	return func() {
		clearTables()
	}
}

func TestChangeUserPasswordSettings(t *testing.T) {
	tests := []struct {
		name             string
		method           string
		url              string
		contentType      string
		body             string
		expectedStatus   int
		expectedBody     string
		expectedPassword string
	}{
		{
			name:           "should validate content type",
			method:         "POST",
			contentType:    "application/xml",
			url:            "/api/me/change-password",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Invalid content type, expected application/jsonInvalid json request body: EOF",
		},
		{
			name:           "should return 400 when json body is not valid",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/me/change-password",
			body:           "<foo />",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid json request body: invalid character '<' looking for beginning of value",
		},
		{
			name:           "should return 400 when request body doesn't have one of mandatory fields",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/me/change-password",
			body:           `{"oldPassword":"","newPassword":""}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `[{"key":"OldPassword","message":"is required"},{"key":"NewPassword","message":"is required"}]`,
		},
		{
			name:           "should return 400 when old password is not valid",
			method:         "POST",
			contentType:    "application/json",
			url:            "/api/me/change-password",
			body:           `{"oldPassword":"wrong_password","newPassword":"new_password"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `[{"key":"oldPassword","message":"Specified old password is invalid"}]`,
		},
		{
			name:             "should return 200 when operation successful",
			method:           "POST",
			contentType:      "application/json",
			url:              "/api/me/change-password",
			body:             `{"oldPassword":"pass","newPassword":"new_password"}`,
			expectedStatus:   http.StatusOK,
			expectedBody:     "",
			expectedPassword: "new_password",
		},
	}

	teardownTests := setupChangeUserPasswordTests()
	defer teardownTests()

	for _, tc := range tests {
		name := fmt.Sprintf("test case: TestChangeUserPasswordSettings %s", tc.name)
		req, err := http.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.contentType)

		require.NoError(t, err)

		sql := sqlx.NewDb(db, "mysql")
		u := tradedb.NewUsers(sql)

		w := httptest.NewRecorder()
		server := &HTTPServer{users: u, log: logger.InitLogger()}
		server.validate = validator.New()
		server.authenticator = &FakeAuthenticator{}

		stubAuthHeader := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("name", "testuser")
				h.ServeHTTP(w, r)
			})
		}

		handler := server.setupRouter(stubAuthHeader)
		handler.ServeHTTP(w, req)

		require.Equal(t, tc.expectedStatus, w.Code, name)
		require.Equal(t, tc.expectedBody, strings.TrimSuffix(w.Body.String(), "\n"), name)

		if tc.expectedPassword != "" {
			a := tradedb.NewAuthenticator(sql)
			require.NoError(t, a.VerifyPassword("testuser", tc.expectedPassword))
		}
	}
}
