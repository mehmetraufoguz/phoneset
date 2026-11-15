package control

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mehmetraufoguz/phoneset/pc/internal/camera"
)

// cameraState answers with the camera as the system shows it now.
func (a *api) cameraState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, a.h.CameraMonitor().Refresh())
}

// cameraReport takes what the part that carries the picture says about itself.
func (a *api) cameraReport(w http.ResponseWriter, r *http.Request) {
	var rep camera.Report
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&rep); err != nil {
		http.Error(w, "bad report", http.StatusBadRequest)
		return
	}
	a.h.CameraMonitor().Report(rep)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) cameraInstall(w http.ResponseWriter, _ *http.Request) {
	exe, _ := os.Executable()
	if err := a.h.CameraMonitor().Backend().Install(filepath.Dir(exe)); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *api) cameraUninstall(w http.ResponseWriter, _ *http.Request) {
	if err := a.h.CameraMonitor().Backend().Uninstall(); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
