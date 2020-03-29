package controllers

import (
	"context"
	"encoding/json"
	"mrkt/constants"
	"mrkt/handlers"
	"mrkt/models"
	"net/http"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/gorilla/mux"

	validator "gopkg.in/go-playground/validator.v9"
)

var defaultRes = make(map[string]interface{})

type loginBody struct {
	Email    string
	Password string
}

type loginResponse struct {
	Message string      `json:"message" bson:"message"`
	Data    interface{} `json:"data" bson:"data"`
}

// AdminController ...
type AdminController struct{}

// CreateUserEndpoint ...
func (c AdminController) CreateUserEndpoint(response http.ResponseWriter, request *http.Request) {

	user := models.GetDefaultUser()
	err := json.NewDecoder(request.Body).Decode(&user)
	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	user.IsAdmin = request.URL.Query().Get("isAdmin") == "true"

	if ok, errors := validateRequest(user); !ok {
		SendErrorResponse(response, http.StatusBadRequest, constants.InvalidParams, errors)
		return
	}

	result, err := handlers.CreateUser(user)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	SendSuccessResponse(response, result)
}

// AdminLoginEndpoint ...
func (c AdminController) AdminLoginEndpoint(response http.ResponseWriter, request *http.Request) {
	var body loginBody

	err := json.NewDecoder(request.Body).Decode(&body)

	if err != nil {
		SendErrorResponse(response, http.StatusBadRequest, err.Error(), defaultRes)
		return
	}

	user, err := handlers.GetUserByEmail(body.Email, true)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	// compare passwords
	if correct := handlers.ComparePasswords(user.Password, []byte(body.Password)); correct != true {
		SendErrorResponse(response, http.StatusUnauthorized, constants.IncorrectCredentials, defaultRes)
		return
	}

	// generate jwt token and send
	token, err := handlers.GenerateJWTToken(&user)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	data := make(map[string]string)
	data["token"] = token

	SendSuccessResponse(response, data)
}

// UpdateUserEndpoint ...
func (c AdminController) UpdateUserEndpoint(response http.ResponseWriter, request *http.Request) {
	// get ID
	params := mux.Vars(request)

	isAdmin := request.URL.Query().Get("isAdmin") == "true"
	user, err := handlers.GetUserByID(params["id"], isAdmin)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	err = json.NewDecoder(request.Body).Decode(&user)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	// update model
	result, err := handlers.UpdateUserByID(params["id"], user)
	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	SendSuccessResponse(response, result)
}

// DeleteUserEndpoint ...
func (c AdminController) DeleteUserEndpoint(response http.ResponseWriter, request *http.Request) {
	// get ID
	params := mux.Vars(request)

	isAdmin := request.URL.Query().Get("isAdmin") == "true"
	user, err := handlers.GetUserByID(params["id"], isAdmin)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	// update model
	result, err := handlers.DeleteUserByID(user)
	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}

	// send reponse
	SendSuccessResponse(response, result)
}

// GetUsersEndpoint ...
func (c AdminController) GetUsersEndpoint(response http.ResponseWriter, request *http.Request) {
	isAdmin := request.URL.Query().Get("isAdmin") == "true"
	results, err := handlers.GetAllUsers(isAdmin)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}
	SendSuccessResponse(response, results)
}

// GetUserEndpoint ...
func (c AdminController) GetUserEndpoint(response http.ResponseWriter, request *http.Request) {
	params := mux.Vars(request)

	isAdmin := request.URL.Query().Get("isAdmin") == "true"
	user, err := handlers.GetUserByID(params["id"], isAdmin)

	if err != nil {
		SendErrorResponse(response, http.StatusInternalServerError, err.Error(), defaultRes)
		return
	}
	SendSuccessResponse(response, user)
}

// AdminAuthenticationMiddleware is a Middleware function, which will be called for each request
func (c AdminController) AdminAuthenticationMiddleware(next http.Handler) http.Handler {

	unauthenticated := []string{"/admin/login"}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// ensure we are not validating an unauthenticated route
		url := r.URL.String()
		if yes := contains(unauthenticated, url); yes {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("Authorization")

		if token == "" {
			SendErrorResponse(w, http.StatusForbidden, constants.AccessDenied, defaultRes)
			return
		}

		if valid, claim := handlers.VerifyJWTToken(token, true); valid {
			// Pass down the request to the next middleware (or final handler)
			ctx := context.WithValue(r.Context(), "AdminID", claim.UserID) // nolint
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			// Write an error and stop the handler chain
			SendErrorResponse(w, http.StatusForbidden, constants.AccessDenied, defaultRes)
		}
	})
}

// SendErrorResponse ...
func SendErrorResponse(r http.ResponseWriter, status int, message string, data interface{}) {

	res := make(map[string]interface{})
	res["status"] = "error"
	res["message"] = message
	res["data"] = data

	jsonRes, _ := json.Marshal(res)

	r.Header().Set("content-type", "application/json")
	r.WriteHeader(status)
	r.Write([]byte(jsonRes))

	if status == http.StatusInternalServerError {
		handlers.ErrorLogger.Error(message)
	}
}

// SendSuccessResponse ...
func SendSuccessResponse(r http.ResponseWriter, result interface{}) {

	res := make(map[string]interface{})
	res["status"] = "success"
	res["data"] = result

	r.Header().Set("content-type", "application/json")
	json.NewEncoder(r).Encode(res)
}

func validateRequest(b interface{}) (bool, map[string]interface{}) {

	// init validator
	v := validator.New()

	// register translations
	translator := en.New()
	uni := ut.New(translator, translator)

	trans, _ := uni.GetTranslator("en")

	_ = v.RegisterTranslation("email", trans, func(ut ut.Translator) error {
		return ut.Add("email", "{0} must be a valid email", true) // see universal-translator for details
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("email", fe.Field())
		return t
	})

	// prepare response
	ok := true
	errors := make(map[string]interface{})

	// parse validation
	err := v.Struct(b)

	if err != nil {
		ok = false
		for _, e := range err.(validator.ValidationErrors) {
			tg := e.Tag()
			errors[tg] = e.Translate(trans)
		}
	}

	return ok, errors
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}
