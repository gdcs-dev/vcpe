package observability

import "encoding/json"

func EncodeMetrics(metrics any) (string, error) {
	b, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
