package routes

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxGatewayRequestBodyBytes = 16 << 20

var errRequestBodyTooLarge = errors.New("request body too large")

func gatewayRequestBodyError(err error) (int, string) {
	if errors.Is(err, errRequestBodyTooLarge) {
		return http.StatusRequestEntityTooLarge, err.Error()
	}
	return http.StatusBadRequest, "Invalid request format"
}

func readGatewayRequestBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxGatewayRequestBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxGatewayRequestBodyBytes {
		return nil, fmt.Errorf("%w: maximum size is %d bytes", errRequestBodyTooLarge, maxGatewayRequestBodyBytes)
	}
	return data, nil
}
