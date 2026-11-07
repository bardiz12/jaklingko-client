package crypto

import (
	"encoding/json"
	"testing"
)

func TestCipherEngine_RoundTrip(t *testing.T) {
	engine := NewCipherEngine(DefaultSalt)

	plain := `{"username":"08123456789","password":"secret"}`
	encWrapper, err := engine.EncryptPayload(plain)
	if err != nil {
		t.Fatalf("EncryptPayload failed: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(encWrapper), &parsed); err != nil {
		t.Fatalf("Failed to parse encrypted wrapper: %v", err)
	}

	reqVal, ok := parsed["request"]
	if !ok || reqVal == "" {
		t.Fatalf("Expected 'request' field in output: %s", encWrapper)
	}

	// Prepare simulated server response with encrypted payload
	simulatedResp := `{"statusCode":200,"message":"Success","response":"` + reqVal + `"}`
	decryptedBytes, err := engine.DecryptPayload([]byte(simulatedResp))
	if err != nil {
		t.Fatalf("DecryptPayload failed: %v", err)
	}

	var resMap map[string]interface{}
	if err := json.Unmarshal(decryptedBytes, &resMap); err != nil {
		t.Fatalf("Failed to unmarshal decrypted response: %v, raw: %s", err, string(decryptedBytes))
	}

	if resMap["username"] != "08123456789" {
		t.Errorf("Expected username '08123456789', got: %v", resMap["username"])
	}
	if resMap["statusCode"] != float64(200) {
		t.Errorf("Expected statusCode 200, got: %v", resMap["statusCode"])
	}
}
