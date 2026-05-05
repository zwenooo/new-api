package common

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"one-api/constant"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const KeyRequestBody = "key_request_body"
const KeyBodyStorage = "key_body_storage"

var ErrRequestBodyTooLarge = errors.New("request body too large")

func IsRequestBodyTooLargeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRequestBodyTooLarge) {
		return true
	}
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

func RequestBodyErrorStatusCode(err error) int {
	if IsRequestBodyTooLargeError(err) {
		return http.StatusRequestEntityTooLarge
	}
	if errors.Is(err, ErrStorageClosed) {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

func GetRequestBody(c *gin.Context) ([]byte, error) {
	storage, err := GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	return storage.Bytes()
}

func GetBodyStorage(c *gin.Context) (BodyStorage, error) {
	if storage, exists := c.Get(KeyBodyStorage); exists && storage != nil {
		bodyStorage, ok := storage.(BodyStorage)
		if !ok {
			return nil, fmt.Errorf("unexpected body storage type: %T", storage)
		}
		if _, err := bodyStorage.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek body storage: %w", err)
		}
		return bodyStorage, nil
	}

	if requestBody, exists := c.Get(KeyRequestBody); exists && requestBody != nil {
		if bodyBytes, ok := requestBody.([]byte); ok {
			bodyStorage, err := CreateBodyStorage(bodyBytes)
			if err != nil {
				return nil, err
			}
			c.Set(KeyBodyStorage, bodyStorage)
			c.Set(KeyRequestBody, nil)
			return bodyStorage, nil
		}
	}

	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	maxBytes := int64(maxMB) << 20
	contentLength := c.Request.ContentLength

	bodyStorage, err := CreateBodyStorageFromReader(c.Request.Body, contentLength, maxBytes)
	if c.Request.Body != nil {
		_ = c.Request.Body.Close()
	}
	if err != nil {
		if IsRequestBodyTooLargeError(err) {
			return nil, fmt.Errorf("%w: request body exceeds %d MB", ErrRequestBodyTooLarge, maxMB)
		}
		return nil, err
	}

	c.Set(KeyBodyStorage, bodyStorage)
	return bodyStorage, nil
}

func CleanupBodyStorage(c *gin.Context) {
	if storage, exists := c.Get(KeyBodyStorage); exists && storage != nil {
		if bodyStorage, ok := storage.(BodyStorage); ok {
			_ = bodyStorage.Close()
		}
	}
	c.Set(KeyBodyStorage, nil)
	c.Set(KeyRequestBody, nil)
}

func UnmarshalBodyReusable(c *gin.Context, v any) error {
	bodyStorage, err := GetBodyStorage(c)
	if err != nil {
		return err
	}
	requestBody, err := bodyStorage.Bytes()
	if err != nil {
		return err
	}
	//if DebugEnabled {
	//	println("UnmarshalBodyReusable request body:", string(requestBody))
	//}
	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		err = Unmarshal(requestBody, v)
	} else if strings.Contains(contentType, gin.MIMEPOSTForm) {
		err = parseFormData(requestBody, v)
	} else if strings.Contains(contentType, gin.MIMEMultipartPOSTForm) {
		err = parseMultipartFormData(c, requestBody, v)
	} else {
		// skip for now
		// TODO: someday non json request have variant model, we will need to implementation this
	}
	if err != nil {
		return err
	}
	if _, err := bodyStorage.Seek(0, io.SeekStart); err != nil {
		return err
	}
	c.Request.Body = io.NopCloser(bodyStorage)
	return nil
}

func ParseMultipartFormReusable(c *gin.Context) (*multipart.Form, error) {
	bodyStorage, err := GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	requestBody, err := bodyStorage.Bytes()
	if err != nil {
		return nil, err
	}

	contentType := c.Request.Header.Get("Content-Type")
	if saved, ok := c.Get("_original_multipart_ct"); ok {
		if savedContentType, ok := saved.(string); ok && strings.TrimSpace(savedContentType) != "" {
			contentType = savedContentType
		}
	} else {
		c.Set("_original_multipart_ct", contentType)
	}

	boundary, err := parseBoundary(contentType)
	if err != nil {
		return nil, err
	}

	reader := multipart.NewReader(bytes.NewReader(requestBody), boundary)
	form, err := reader.ReadForm(multipartMemoryLimit())
	if err != nil {
		return nil, err
	}

	c.Request.MultipartForm = form
	if _, err := bodyStorage.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bodyStorage)
	return form, nil
}

var errBoundaryNotFound = errors.New("multipart boundary not found")

func parseBoundary(contentType string) (string, error) {
	if strings.TrimSpace(contentType) == "" {
		return "", errBoundaryNotFound
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return "", errBoundaryNotFound
	}
	return boundary, nil
}

func multipartMemoryLimit() int64 {
	limitMB := constant.MaxFileDownloadMB
	if limitMB <= 0 {
		limitMB = 32
	}
	return int64(limitMB) << 20
}

func processFormMap(formMap map[string]any, v any) error {
	jsonData, err := Marshal(formMap)
	if err != nil {
		return err
	}
	return Unmarshal(jsonData, v)
}

func parseFormData(data []byte, v any) error {
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	formMap := make(map[string]any)
	for key, vals := range values {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}
	return processFormMap(formMap, v)
}

func parseMultipartFormData(c *gin.Context, data []byte, v any) error {
	contentType := c.Request.Header.Get("Content-Type")
	if saved, ok := c.Get("_original_multipart_ct"); ok {
		if savedContentType, ok := saved.(string); ok && strings.TrimSpace(savedContentType) != "" {
			contentType = savedContentType
		}
	} else {
		c.Set("_original_multipart_ct", contentType)
	}

	boundary, err := parseBoundary(contentType)
	if err != nil {
		if errors.Is(err, errBoundaryNotFound) {
			return Unmarshal(data, v)
		}
		return err
	}

	reader := multipart.NewReader(bytes.NewReader(data), boundary)
	form, err := reader.ReadForm(multipartMemoryLimit())
	if err != nil {
		return err
	}
	defer form.RemoveAll()

	formMap := make(map[string]any)
	for key, vals := range form.Value {
		if len(vals) == 1 {
			formMap[key] = vals[0]
		} else {
			formMap[key] = vals
		}
	}
	return processFormMap(formMap, v)
}

func SetContextKey(c *gin.Context, key constant.ContextKey, value any) {
	c.Set(string(key), value)
}

func GetContextKey(c *gin.Context, key constant.ContextKey) (any, bool) {
	return c.Get(string(key))
}

func GetContextKeyString(c *gin.Context, key constant.ContextKey) string {
	return c.GetString(string(key))
}

func GetContextKeyInt(c *gin.Context, key constant.ContextKey) int {
	return c.GetInt(string(key))
}

func GetContextKeyBool(c *gin.Context, key constant.ContextKey) bool {
	return c.GetBool(string(key))
}

func GetContextKeyFloat64(c *gin.Context, key constant.ContextKey) float64 {
	if value, ok := c.Get(string(key)); ok {
		switch v := value.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func GetContextKeyStringSlice(c *gin.Context, key constant.ContextKey) []string {
	return c.GetStringSlice(string(key))
}

func GetContextKeyStringMap(c *gin.Context, key constant.ContextKey) map[string]any {
	return c.GetStringMap(string(key))
}

func GetContextKeyTime(c *gin.Context, key constant.ContextKey) time.Time {
	return c.GetTime(string(key))
}

func GetContextKeyType[T any](c *gin.Context, key constant.ContextKey) (T, bool) {
	if value, ok := c.Get(string(key)); ok {
		if v, ok := value.(T); ok {
			return v, true
		}
	}
	var t T
	return t, false
}

func ApiError(c *gin.Context, err error) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": err.Error(),
	})
}

func ApiErrorMsg(c *gin.Context, msg string) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": msg,
	})
}

func ApiSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}
