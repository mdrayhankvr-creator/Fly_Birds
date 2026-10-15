package httpapi

import "github.com/gin-gonic/gin"

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func success(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func failure(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": errorBody{Code: code, Message: message}})
}
