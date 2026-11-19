package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

// maxProfileNameLength mirrors the limit WhatsApp's own apps enforce on the
// profile (push) name.
const maxProfileNameLength = 25

type profileNameRequest struct {
	Name string
}

func (t profileNameRequest) name() (string, error) {
	name := strings.TrimSpace(t.Name)
	if name == "" {
		return "", errors.New("missing Name in Payload")
	}
	if utf8.RuneCountInString(name) > maxProfileNameLength {
		return "", fmt.Errorf("Name must be at most %d characters", maxProfileNameLength)
	}
	return name, nil
}

type profilePhotoRequest struct {
	Image string
}

// jpeg returns the decoded photo, or nil when Image is empty, which removes
// the current photo.
func (t profilePhotoRequest) jpeg() ([]byte, error) {
	if t.Image == "" {
		return nil, nil
	}
	if !strings.HasPrefix(t.Image, "data:image") {
		return nil, errors.New("Image should be a data URL starting with \"data:image/jpeg\", or empty to remove the photo")
	}
	decoded, err := dataurl.DecodeString(t.Image)
	if err != nil {
		return nil, errors.New("could not decode base64 encoded data from payload")
	}
	data := decoded.Data
	// WhatsApp only accepts JPEG profile photos.
	if len(data) < 3 || data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
		return nil, errors.New("image must be in JPEG format")
	}
	return data, nil
}

// SetProfileName changes the account's own profile (push) name. whatsmeow has
// no direct setter; the name is a setting_pushName app-state mutation, the
// same one whatsmeow decodes when the phone changes it.
func (s *server) SetProfileName() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request profileNameRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		name, err := request.name()
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		client := clientManager.GetWhatsmeowClient(r.Context().Value("userinfo").(Values).Get("Id"))
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		if err = client.SendAppState(ctx, appstate.BuildSettingPushName(name)); err != nil {
			// App-state keys arrive from the phone after pairing; until then
			// there is nothing to encrypt the patch with.
			if strings.Contains(err.Error(), "no app state keys found") {
				s.Respond(w, r, http.StatusConflict, errors.New("app state keys not synced yet; wait for the initial sync after pairing and retry"))
				return
			}
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("error setting profile name: %w", err))
			return
		}
		// whatsmeow only updates the stored push name when it receives the
		// mutation, not when it sends one.
		client.Store.PushName = name
		if err = client.Store.Save(ctx); err != nil {
			log.Warn().Err(err).Msg("Profile name set but failed to save device store")
		}

		responseJson, err := json.Marshal(map[string]interface{}{"Details": "Profile name set", "Name": name})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

// SetProfilePhoto changes or removes the account's own profile photo. It is
// the same w:profile:picture query as a group photo, sent without a target,
// which WhatsApp applies to the sender's own profile.
func (s *server) SetProfilePhoto() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request profilePhotoRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		photo, err := request.jpeg()
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, err)
			return
		}
		client := clientManager.GetWhatsmeowClient(r.Context().Value("userinfo").(Values).Get("Id"))
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		pictureID, err := client.SetGroupPhoto(ctx, types.EmptyJID, photo)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, fmt.Errorf("error setting profile photo: %w", err))
			return
		}

		details := "Profile photo set"
		if photo == nil {
			details = "Profile photo removed"
		}
		responseJson, err := json.Marshal(map[string]interface{}{"Details": details, "PictureID": pictureID})
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}
