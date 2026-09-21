package systemone

import (
	"encoding/json"
	"testing"
)

func TestParseRequest(t *testing.T) {
	valid := `{"model":"jev-latest","state":{"text":"refund"},"questions":{"n":{"type":"noul","instructions":"Refund?"},"c":{"type":"choice","instructions":{"question":"Team?"},"criteria":{"billing":null,"other":["Other"]}},"s":{"type":"score","instructions":"Urgency?","criteria":["Low","High"]}}}`
	req, err := ParseRequest([]byte(valid))
	if err != nil || len(req.Questions) != 3 {
		t.Fatalf("parse: %v", err)
	}
	encoded, _ := json.Marshal(req)
	var before, after any
	_ = json.Unmarshal([]byte(valid), &before)
	_ = json.Unmarshal(encoded, &after)
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatalf("payload changed: %s", a)
	}
	for _, body := range []string{
		`{}`, `null`, valid + `{}`, `{"model":"jev","state":1,"questions":{}}`,
		`{"model":"jev","state":"x","questions":{"q":{"type":"choice","instructions":"x"}}}`,
		`{"model":"jev","state":"x","questions":{"q":{"type":"score","instructions":"x","criteria":["x"]}}}`,
		`{"model":"jev","state":"x","questions":{"q":{"type":"noul","instructions":null}}}`,
		`{"model":"jev","state":"x","questions":{"q":{"type":"noul","instructions":"x"}},"stream":false}`,
	} {
		if _, err := ParseRequest([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestValidateResponseRejectsIncompleteSuccess(t *testing.T) {
	req, _ := ParseRequest([]byte(`{"model":"jev","state":"x","questions":{"n":{"type":"noul","instructions":"x"}}}`))
	for _, body := range []string{
		`{}`, `null`,
		`{"model":"jev","answers":{"n":{"type":"noul","noul":0.9}}}`,
		`{"model":"jev","answers":{"n":{"type":"noul","noul":null}},"usage":{"input_tokens":0,"output_tokens":0}}`,
		`{"model":"jev","answers":{"n":{"type":"noul","noul":1.1}},"usage":{"input_tokens":0,"output_tokens":0}}`,
		`{"model":"jev","answers":{"other":{"type":"noul","noul":0.9}},"usage":{"input_tokens":0,"output_tokens":0}}`,
	} {
		if _, err := ValidateResponse([]byte(body), req); err == nil {
			t.Errorf("accepted incomplete response: %s", body)
		}
	}
}
