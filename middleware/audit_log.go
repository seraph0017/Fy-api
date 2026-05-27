package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

const auditBodyMaxBytes = 4096

// AuditLog records admin/root write operations to the application log.
// Fy-api overlay: admin operation audit logging
func AuditLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		action := methodToAction(method)
		resource, resourceId := parseResource(path, c)

		// For option updates, capture old value before handler executes
		var oldOptionValue string
		isOptionUpdate := (resource == "option" && action == "update")
		if isOptionUpdate {
			oldOptionValue = captureOldOptionValue(c)
		}

		// Cache request body for detail logging
		bodySnippet := readBodySnippet(c)

		c.Next()

		// Only log successful operations
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			userId := c.GetInt("id")
			username, _ := c.Get("username")
			usernameStr := fmt.Sprintf("%v", username)

			detail := buildDetail(isOptionUpdate, oldOptionValue, bodySnippet, resource, method, path)

			logLine := fmt.Sprintf("[AUDIT] user_id=%d username=%s action=%s resource=%s resource_id=%s ip=%s time=%s",
				userId, usernameStr, action, resource, resourceId, c.ClientIP(),
				time.Now().Format("2006-01-02T15:04:05Z07:00"))
			if detail != "" {
				logLine += " detail=" + detail
			}
			logger.LogInfo(c, logLine)
		}
	}
}

func methodToAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return method
	}
}

func parseResource(path string, c *gin.Context) (resource string, resourceId string) {
	// /api/channel/5 → resource=channel, resourceId=5
	// /api/option → resource=option, resourceId from body key
	parts := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
	if len(parts) > 0 {
		resource = parts[0]
	}
	if len(parts) > 1 && parts[1] != "" {
		resourceId = parts[1]
	}
	if id := c.Param("id"); id != "" {
		resourceId = id
	}
	return
}

func captureOldOptionValue(c *gin.Context) string {
	body, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	var req struct {
		Key string `json:"key"`
	}
	_ = common.Unmarshal(body, &req)
	if req.Key == "" {
		return ""
	}
	common.OptionMapRWMutex.RLock()
	val := common.OptionMap[req.Key]
	common.OptionMapRWMutex.RUnlock()
	return val
}

func readBodySnippet(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if len(body) > auditBodyMaxBytes {
		return string(body[:auditBodyMaxBytes]) + "...(truncated)"
	}
	return string(body)
}

func buildDetail(isOptionUpdate bool, oldValue, bodySnippet, _, method, path string) string {
	if isOptionUpdate && bodySnippet != "" {
		var req struct {
			Key   string `json:"key"`
			Value any    `json:"value"`
		}
		_ = common.Unmarshal([]byte(bodySnippet), &req)
		if req.Key != "" {
			newVal := fmt.Sprintf("%v", req.Value)
			return fmt.Sprintf(`{"key":"%s","old_value":"%s","new_value":"%s"}`,
				req.Key, escapeJsonValue(oldValue), escapeJsonValue(newVal))
		}
	}
	if bodySnippet != "" {
		return fmt.Sprintf(`{"method":"%s","path":"%s","body_size":%d}`,
			method, path, len(bodySnippet))
	}
	return ""
}

func escapeJsonValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
