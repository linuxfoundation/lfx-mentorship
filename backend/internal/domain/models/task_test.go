// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/linuxfoundation/lfx-v2-mentorship-service/internal/domain/models"
)

func TestTaskJSONNeverCarriesTheObjectKey(t *testing.T) {
	key := "0b9a3f4e-6d5c-4b3a-9f8e-7d6c5b4a3f2e-essay.pdf"
	data, err := json.Marshal(&models.Task{ID: "t1", AssigneeID: "u1", File: &key})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), key) {
		t.Fatalf("response leaks the object key: %s", data)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["file"] != "/mentorship/v1/tasks/t1/file-download" || body["assignee_id"] != "u1" {
		t.Fatalf("body = %v", body)
	}

	data, err = json.Marshal(models.Task{ID: "t2"})
	if err != nil || strings.Contains(string(data), `"file"`) {
		t.Fatalf("a task without a file must omit it: %s (err %v)", data, err)
	}
}
