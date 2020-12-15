package api

import (
	"net/http"

	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/logger"
	"github.com/lulugyf/sshserv/serv"
	"github.com/lulugyf/sshserv/utils"
	"github.com/go-chi/render"
)

func getQuotaScans(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, serv.GetQuotaScans())
}

func startQuotaScan(w http.ResponseWriter, r *http.Request) {
	var u dataprovider.User
	err := render.DecodeJSON(r.Body, &u)
	if err != nil {
		sendAPIResponse(w, r, err, "", http.StatusBadRequest)
		return
	}
	user, err := dataprovider.UserExists(dataProvider, u.Username)
	if err != nil {
		sendAPIResponse(w, r, err, "", http.StatusNotFound)
		return
	}
	if serv.AddQuotaScan(user.Username) {
		sendAPIResponse(w, r, err, "Scan started", http.StatusCreated)
		go func() {
			numFiles, size, _, err := utils.ScanDirContents(user.HomeDir)
			if err != nil {
				logger.Warn(logSender, "error scanning user home dir %v: %v", user.HomeDir, err)
			} else {
				err := dataprovider.UpdateUserQuota(dataProvider, user, numFiles, size, true)
				logger.Debug(logSender, "user dir scanned, user: %v, dir: %v, error: %v", user.Username, user.HomeDir, err)
			}
			serv.RemoveQuotaScan(user.Username)
		}()
	} else {
		sendAPIResponse(w, r, err, "Another scan is already in progress", http.StatusConflict)
	}
}
