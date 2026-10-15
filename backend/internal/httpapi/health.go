package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type HealthData struct {
	Status string `json:"status"`
}

type HealthResponse struct {
	Data HealthData `json:"data"`
}

// healthz godoc
// @Summary Check API liveness
// @Description Returns the API process status; it does not check database readiness.
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /healthz [get]
func healthz(c *gin.Context) {
	success(c, http.StatusOK, HealthData{Status: "ok"})
}
