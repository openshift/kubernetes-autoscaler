package errors

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

var jsonUnescaper = strings.NewReplacer(
	"\n", " ",
	"\t", " ",
	"\r", " ",
)

type ResponseErrorWrapper struct {
	respErr *azcore.ResponseError
	message string
}

func NewResponseErrorWrapper(respErr *azcore.ResponseError) *ResponseErrorWrapper {
	return &ResponseErrorWrapper{
		respErr: respErr,
	}
}

func (e *ResponseErrorWrapper) Unwrap() error {
	return e.respErr
}

// WrapResponseError wraps ResponseError instances in ResponseErrorWrapper for more concise formatting.
// If the error is a ResponseError, it returns a wrapped version which has a more concise .Error() output.
// If the error is not a ResponseError, it returns the original error unchanged.
func WrapResponseError(err error) error {
	if azErr := IsResponseError(err); azErr != nil {
		return NewResponseErrorWrapper(azErr)
	}
	return err
}

func (c *ResponseErrorWrapper) Error() string {
	if c.message != "" {
		return c.message
	}

	if c.respErr == nil {
		// TODO - special handling if this is nil? But for now, just return empty string to not pollute logs
		return ""
	}

	// Attempt to build error message - this is best effort since format can vary depending on the Azure service
	c.message = buildWrapperErrorMessage(c.respErr)

	return c.message
}

func buildWrapperErrorMessage(respErr *azcore.ResponseError) string {
	httpCode := respErr.StatusCode
	errorCode := respErr.ErrorCode
	if errorCode == "" {
		errorCode = "UNAVAILABLE"
	}

	// Extract HTTP Method and URL
	httpMethod, url := extractRequestInfo(respErr)

	// Extract error message
	errorMessage := extractErrorMessage(respErr)

	wrapperMessage := fmt.Sprintf("HTTP CODE: %d, ERROR CODE: %s, MESSAGE: %s, REQUEST: %s %s",
		httpCode, errorCode, errorMessage, httpMethod, url)

	return wrapperMessage
}

// extractRequestInfo extracts HTTP method and URL with proper nil checks
func extractRequestInfo(respErr *azcore.ResponseError) (string, string) {
	method := "UNKNOWN"
	requestURL := "UNAVAILABLE"

	if respErr.RawResponse == nil || respErr.RawResponse.Request == nil {
		return method, requestURL
	}

	req := respErr.RawResponse.Request
	method = req.Method

	if req.URL != nil {
		requestURL = req.URL.String()
	}

	return method, requestURL
}

type AzureErrorResponse struct {
	Error   AzureError `json:"error"`
	Code    string     `json:"code"`
	Message string     `json:"message"`
	Details any        `json:"details"`
}

type AzureError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}

func extractErrorMessage(respErr *azcore.ResponseError) string {
	// these 2 cases shouldn't happen in real-world scenarios as a
	// response with no body should set it to http.NoBody
	if respErr.RawResponse == nil {
		return "UNAVAILABLE"
	}

	if respErr.RawResponse.Body == nil {
		return "UNAVAILABLE"
	}

	// Read the body content once and save it in case we need to use one of the fallback approaches for message extraction
	respBody := respErr.RawResponse.Body
	bodyBytes, err := io.ReadAll(respBody)
	if err != nil {
		return "UNAVAILABLE"
	}

	var result AzureErrorResponse
	err = json.Unmarshal(bodyBytes, &result)
	if err != nil {
		return "UNAVAILABLE"
	}

	// Check wrapped format first (with "error" wrapper, seems to be more common)
	if result.Error.Message != "" {
		return jsonUnescaper.Replace(result.Error.Message)
	}

	// Check unwrapped format (without "error" wrapper)
	if result.Message != "" {
		return jsonUnescaper.Replace(result.Message)
	}

	// If no message found, return unavailable
	return "UNAVAILABLE"
}
