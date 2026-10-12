package smoke

import "testing"

func TestSmokeRun(t *testing.T) {
	result, err := Run()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || !result.UnauthedDenied || !result.LoginSucceeded ||
		!result.DashboardOK || !result.DeviceAPI || !result.CSRFEnforced ||
		!result.LogoutRevoked || !result.ShutdownOK {
		t.Fatalf("incomplete HTTP self-test: %+v", result)
	}
	if result.DeviceModel == "" || result.Kernel == "" {
		t.Fatal("no kernel data")
	}
	if result.Listener != "127.0.0.1:ephemeral" {
		t.Fatal("unexpected bind")
	}
}
