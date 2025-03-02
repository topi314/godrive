package godrive

import (
	"errors"
	"net/http"

	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/templates"
)

func (s *Server) PostToken(w http.ResponseWriter, r *http.Request) {
	userInfo := auth.GetUserInfo(r)
	err := r.ParseForm()
	if err != nil {
		s.prettyError(w, r, err, http.StatusBadRequest)
		return
	}

	description := r.Form.Get("description")
	if description == "" {
		s.prettyError(w, r, errors.New("missing description"), http.StatusBadRequest)
		return
	}

	token := s.auth.NewID(32)
	apiToken, err := s.db.CreateToken(r.Context(), token, userInfo.Subject, description)
	if err != nil {
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	templateApiToken := templates.ApiToken{
		Description: apiToken.Description,
		Token:       apiToken.Token,
	}

	templates.SettingsTokenEntry(templateApiToken)
	return
}

func (s *Server) DeleteToken(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get(auth.AuthorizationHeader)
	if token == "" {
		s.prettyError(w, r, errors.New("missing authorization header"), http.StatusBadRequest)
		return
	}

	err := s.db.DeleteApiToken(r.Context(), token)
	if err != nil {
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	return
}
