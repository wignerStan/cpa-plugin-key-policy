package plugin

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSchedulerAdmissionSharesGroupAndWeightWithoutPicking(t *testing.T) {
	app, _ := configureTestApp(t)
	control, _ := configureTestApp(t)
	req := SchedulerPickRequest{
		Provider: "codex", Model: "model", Options: SchedulerPickOptions{Metadata: map[string]any{"group": "team"}},
		Candidates: []SchedulerAuthCandidate{
			{ID: "a", Provider: "codex", Priority: 10, Attributes: map[string]string{"plan_type": "team", "weight": "2"}},
			{ID: "b", Provider: "codex", Priority: 10, Attributes: map[string]string{"plan_type": "team", "weight": "1"}},
			{ID: "low-cooling", Provider: "codex", Priority: -10, Status: "cooldown", Attributes: map[string]string{"plan_type": "team"}},
			{ID: "paused", Provider: "codex", Priority: 100, Attributes: map[string]string{"plan_type": "team", "weight": "0"}},
			{ID: "other-group", Provider: "codex", Priority: 100, Attributes: map[string]string{"plan_type": "free"}},
		},
	}
	if !app.registration().Capabilities.SchedulerAdmission {
		t.Fatal("admission callback was not registered")
	}
	if schedulerPickForTest(t, app, req).AuthID != schedulerPickForTest(t, control, req).AuthID {
		t.Fatal("different initial schedules")
	}
	raw, _ := json.Marshal(req)
	for i := 0; i < 10; i++ {
		response, err := app.HandleMethod(MethodSchedulerAdmit, raw)
		if err != nil {
			t.Fatal(err)
		}
		var admitted SchedulerAdmissionResponse
		if err = unmarshalOK(response, &admitted); err != nil || !admitted.Handled || !reflect.DeepEqual(admitted.AuthIDs, []string{"a", "b", "low-cooling"}) {
			t.Fatal("admission must include lower/cooling targets and exclude other groups or zero weight", admitted, err)
		}
	}
	for i := 0; i < 12; i++ {
		if schedulerPickForTest(t, app, req).AuthID != schedulerPickForTest(t, control, req).AuthID {
			t.Fatal("admission changed weighted selection state")
		}
	}
}

func TestSchedulerAdmissionNativeKeyAndEmptyGroupAreDistinct(t *testing.T) {
	app, _ := configureTestApp(t)
	for _, group := range []string{"", "unavailable"} {
		req := SchedulerPickRequest{Model: "model", Options: SchedulerPickOptions{Metadata: map[string]any{"group": group}}, Candidates: []SchedulerAuthCandidate{{ID: "a", Provider: "codex"}}}
		raw, _ := json.Marshal(req)
		response, err := app.HandleMethod(MethodSchedulerAdmit, raw)
		if err != nil {
			t.Fatal(err)
		}
		var admitted SchedulerAdmissionResponse
		if err = unmarshalOK(response, &admitted); err != nil || admitted.Handled != (group != "") || len(admitted.AuthIDs) != 0 {
			t.Fatal("native delegation and known empty pool must remain distinct", admitted, err)
		}
	}
}
