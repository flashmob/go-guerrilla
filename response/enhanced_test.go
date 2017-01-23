package response

import "testing"

func TestClass(t *testing.T) {
	if ClassPermanentFailure != 5 {
		t.Error("ClassPermanentFailure is not 5")
	}
	if ClassTransientFailure != 4 {
		t.Error("ClassTransientFailure is not 4")
	}
	if ClassSuccess != 2 {
		t.Error("ClassSuccess is not 2")
	}
}

func TestGetBasicStatusCode(t *testing.T) {
	// Known status code
	a := getBasicStatusCode("2.5.0")
	if a != 250 {
		t.Errorf("getBasicStatusCode. Int \"%d\" not expected.", a)
	}

	// Unknown status code
	b := getBasicStatusCode("2.0.0")
	if b != 200 {
		t.Errorf("getBasicStatusCode. Int \"%d\" not expected.", b)
	}
}

// TestString for the String function
func TestCustomString(t *testing.T) {
	// Basic testing
	a := CustomString(OtherStatus, 200, ClassSuccess, "Test")
	if a != "200 2.0.0 Test" {
		t.Errorf("CustomString failed. String \"%s\" not expected.", a)
	}

	// Default String
	b := String(OtherStatus, ClassSuccess)
	if b != "200 2.0.0 OK" {
		t.Errorf("String failed. String \"%s\" not expected.", b)
	}
}

func TestBuildEnhancedResponseFromDefaultStatus(t *testing.T) {
	a := buildEnhancedResponseFromDefaultStatus(ClassPermanentFailure, InvalidCommand)
	if a != "5.5.1" {
		t.Errorf("buildEnhancedResponseFromDefaultStatus failed. String \"%s\" not expected.", a)
	}
}
