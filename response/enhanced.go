package response

import (
	"fmt"
)

const (
	// ClassSuccess specifies that the DSN is reporting a positive delivery
	// action.  Detail sub-codes may provide notification of
	// transformations required for delivery.
	ClassSuccess = 2
	// ClassTransientFailure - a persistent transient failure is one in which the message as
	// sent is valid, but persistence of some temporary condition has
	// caused abandonment or delay of attempts to send the message.
	// If this code accompanies a delivery failure report, sending in
	// the future may be successful.
	ClassTransientFailure = 4
	// ClassPermanentFailure - a permanent failure is one which is not likely to be resolved
	// by resending the message in the current form.  Some change to
	// the message or the destination must be made for successful
	// delivery.
	ClassPermanentFailure = 5
)

type class int

func (c class) String() string {
	return fmt.Sprintf("%c00", c)
}

// codeMap for mapping Enhanced Status Code to Basic Code
// Mapping according to https://www.iana.org/assignments/smtp-enhanced-status-codes/smtp-enhanced-status-codes.xml
// This might not be entirely useful
var codeMap = struct {
	m map[EnhancedStatus]int
}{m: map[EnhancedStatus]int{

	EnhancedStatus{ClassSuccess, ".1.0"}: 250,
	EnhancedStatus{ClassSuccess, ".1.5"}: 250,
	EnhancedStatus{ClassSuccess, ".3.0"}: 250,
	EnhancedStatus{ClassSuccess, ".5.0"}: 250,
	EnhancedStatus{ClassSuccess, ".6.4"}: 250,
	EnhancedStatus{ClassSuccess, ".6.8"}: 252,
	EnhancedStatus{ClassSuccess, ".7.0"}: 220,

	EnhancedStatus{ClassTransientFailure, ".1.1"}:  451,
	EnhancedStatus{ClassTransientFailure, ".1.8"}:  451,
	EnhancedStatus{ClassTransientFailure, ".2.4"}:  450,
	EnhancedStatus{ClassTransientFailure, ".3.0"}:  421,
	EnhancedStatus{ClassTransientFailure, ".3.1"}:  452,
	EnhancedStatus{ClassTransientFailure, ".3.2"}:  453,
	EnhancedStatus{ClassTransientFailure, ".4.1"}:  451,
	EnhancedStatus{ClassTransientFailure, ".4.2"}:  421,
	EnhancedStatus{ClassTransientFailure, ".4.3"}:  451,
	EnhancedStatus{ClassTransientFailure, ".4.5"}:  451,
	EnhancedStatus{ClassTransientFailure, ".5.0"}:  451,
	EnhancedStatus{ClassTransientFailure, ".5.1"}:  430,
	EnhancedStatus{ClassTransientFailure, ".5.3"}:  452,
	EnhancedStatus{ClassTransientFailure, ".5.4"}:  451,
	EnhancedStatus{ClassTransientFailure, ".7.0"}:  450,
	EnhancedStatus{ClassTransientFailure, ".7.1"}:  451,
	EnhancedStatus{ClassTransientFailure, ".7.12"}: 422,
	EnhancedStatus{ClassTransientFailure, ".7.15"}: 450,
	EnhancedStatus{ClassTransientFailure, ".7.24"}: 451,

	EnhancedStatus{ClassPermanentFailure, ".1.1"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".1.3"}:  501,
	EnhancedStatus{ClassPermanentFailure, ".1.8"}:  501,
	EnhancedStatus{ClassPermanentFailure, ".1.10"}: 556,
	EnhancedStatus{ClassPermanentFailure, ".2.2"}:  552,
	EnhancedStatus{ClassPermanentFailure, ".2.3"}:  552,
	EnhancedStatus{ClassPermanentFailure, ".3.0"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".3.4"}:  552,
	EnhancedStatus{ClassPermanentFailure, ".4.3"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".5.0"}:  501,
	EnhancedStatus{ClassPermanentFailure, ".5.1"}:  500,
	EnhancedStatus{ClassPermanentFailure, ".5.2"}:  500,
	EnhancedStatus{ClassPermanentFailure, ".5.4"}:  501,
	EnhancedStatus{ClassPermanentFailure, ".5.6"}:  500,
	EnhancedStatus{ClassPermanentFailure, ".6.3"}:  554,
	EnhancedStatus{ClassPermanentFailure, ".6.6"}:  554,
	EnhancedStatus{ClassPermanentFailure, ".6.7"}:  553,
	EnhancedStatus{ClassPermanentFailure, ".6.8"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".6.9"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".7.0"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".7.1"}:  551,
	EnhancedStatus{ClassPermanentFailure, ".7.2"}:  550,
	EnhancedStatus{ClassPermanentFailure, ".7.4"}:  504,
	EnhancedStatus{ClassPermanentFailure, ".7.8"}:  554,
	EnhancedStatus{ClassPermanentFailure, ".7.9"}:  534,
	EnhancedStatus{ClassPermanentFailure, ".7.10"}: 523,
	EnhancedStatus{ClassPermanentFailure, ".7.11"}: 524,
	EnhancedStatus{ClassPermanentFailure, ".7.13"}: 525,
	EnhancedStatus{ClassPermanentFailure, ".7.14"}: 535,
	EnhancedStatus{ClassPermanentFailure, ".7.15"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.16"}: 552,
	EnhancedStatus{ClassPermanentFailure, ".7.17"}: 500,
	EnhancedStatus{ClassPermanentFailure, ".7.18"}: 500,
	EnhancedStatus{ClassPermanentFailure, ".7.19"}: 500,
	EnhancedStatus{ClassPermanentFailure, ".7.20"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.21"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.22"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.23"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.24"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.25"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.26"}: 550,
	EnhancedStatus{ClassPermanentFailure, ".7.27"}: 550,
}}

var (
	// Canned is to be read-only, except in the init() function
	Canned Responses
)

// Responses has some already pre-constructed responses
type Responses struct {

	// The 500's
	FailLineTooLong              string
	FailNestedMailCmd            string
	FailNoSenderDataCmd          string
	FailNoRecipientsDataCmd      string
	FailUnrecognizedCmd          string
	FailMaxUnrecognizedCmd       string
	FailReadLimitExceededDataCmd string
	FailMessageSizeExceeded      string
	FailReadErrorDataCmd         string
	FailPathTooLong              string
	FailInvalidAddress           string
	FailLocalPartTooLong         string
	FailDomainTooLong            string
	FailBackendNotRunning        string
	FailBackendTransaction       string
	FailBackendTimeout           string

	// The 400's
	ErrorTooManyRecipients string
	ErrorRelayDenied       string
	ErrorShutdown          string

	// The 200's
	SuccessMailCmd       string
	SuccessRcptCmd       string
	SuccessResetCmd      string
	SuccessVerifyCmd     string
	SuccessNoopCmd       string
	SuccessQuitCmd       string
	SuccessDataCmd       string
	SuccessStartTLSCmd   string
	SuccessMessageQueued string
}

func init() {
	Canned = Responses{}
	Canned.FailLineTooLong = (&Response{
		EnhancedCode: InvalidCommand,
		BasicCode:    554,
		Class:        ClassPermanentFailure,
		Comment:      "Line too long.",
	}).String()

	Canned.FailNestedMailCmd = (&Response{
		EnhancedCode: InvalidCommand,
		BasicCode:    503,
		Class:        ClassPermanentFailure,
		Comment:      "Error: nested MAIL command",
	}).String()

	Canned.SuccessMailCmd = (&Response{
		EnhancedCode: OtherAddressStatus,
		Class:        ClassSuccess,
	}).String()

	Canned.SuccessRcptCmd = (&Response{
		EnhancedCode: DestinationMailboxAddressValid,
		Class:        ClassSuccess,
	}).String()

	Canned.SuccessResetCmd = Canned.SuccessMailCmd
	Canned.SuccessNoopCmd = (&Response{
		EnhancedCode: OtherStatus,
		Class:        ClassSuccess,
	}).String()

	Canned.SuccessVerifyCmd = (&Response{
		EnhancedCode: OtherOrUndefinedProtocolStatus,
		BasicCode:    252,
		Class:        ClassSuccess,
		Comment:      "Cannot verify user",
	}).String()

	Canned.ErrorTooManyRecipients = (&Response{
		EnhancedCode: TooManyRecipients,
		BasicCode:    452,
		Class:        ClassTransientFailure,
		Comment:      "Too many recipients",
	}).String()

	Canned.ErrorRelayDenied = (&Response{
		EnhancedCode: BadDestinationMailboxAddress,
		BasicCode:    454,
		Class:        ClassTransientFailure,
		Comment:      "Error: Relay access denied: ",
	}).String()

	Canned.SuccessQuitCmd = (&Response{
		EnhancedCode: OtherStatus,
		BasicCode:    221,
		Class:        ClassSuccess,
		Comment:      "Bye",
	}).String()

	Canned.FailNoSenderDataCmd = (&Response{
		EnhancedCode: InvalidCommand,
		BasicCode:    503,
		Class:        ClassPermanentFailure,
		Comment:      "Error: No sender",
	}).String()

	Canned.FailNoRecipientsDataCmd = (&Response{
		EnhancedCode: InvalidCommand,
		BasicCode:    503,
		Class:        ClassPermanentFailure,
		Comment:      "Error: No recipients",
	}).String()

	Canned.SuccessDataCmd = "354 Enter message, ending with '.' on a line by itself"

	Canned.SuccessStartTLSCmd = (&Response{
		EnhancedCode: OtherStatus,
		BasicCode:    220,
		Class:        ClassSuccess,
		Comment:      "Ready to start TLS",
	}).String()

	Canned.FailUnrecognizedCmd = (&Response{
		EnhancedCode: InvalidCommand,
		BasicCode:    554,
		Class:        ClassPermanentFailure,
		Comment:      "Unrecognized command",
	}).String()

	Canned.FailMaxUnrecognizedCmd = (&Response{
		EnhancedCode: InvalidCommand,
		BasicCode:    554,
		Class:        ClassPermanentFailure,
		Comment:      "Too many unrecognized commands",
	}).String()

	Canned.ErrorShutdown = (&Response{
		EnhancedCode: OtherOrUndefinedMailSystemStatus,
		BasicCode:    421,
		Class:        ClassTransientFailure,
		Comment:      "Server is shutting down. Please try again later. Sayonara!",
	}).String()

	Canned.FailReadLimitExceededDataCmd = (&Response{
		EnhancedCode: SyntaxError,
		BasicCode:    550,
		Class:        ClassPermanentFailure,
		Comment:      "Error: ",
	}).String()

	Canned.FailMessageSizeExceeded = (&Response{
		EnhancedCode: SyntaxError,
		BasicCode:    550,
		Class:        ClassPermanentFailure,
		Comment:      "Error: ",
	}).String()

	Canned.FailReadErrorDataCmd = (&Response{
		EnhancedCode: OtherOrUndefinedMailSystemStatus,
		BasicCode:    451,
		Class:        ClassTransientFailure,
		Comment:      "Error: ",
	}).String()

	Canned.FailPathTooLong = (&Response{
		EnhancedCode: InvalidCommandArguments,
		BasicCode:    550,
		Class:        ClassPermanentFailure,
		Comment:      "Path too long",
	}).String()

	Canned.FailInvalidAddress = (&Response{
		EnhancedCode: InvalidCommandArguments,
		BasicCode:    501,
		Class:        ClassPermanentFailure,
		Comment:      "Invalid address",
	}).String()

	Canned.FailLocalPartTooLong = (&Response{
		EnhancedCode: InvalidCommandArguments,
		BasicCode:    550,
		Class:        ClassPermanentFailure,
		Comment:      "Local part too long, cannot exceed 64 characters",
	}).String()

	Canned.FailDomainTooLong = (&Response{
		EnhancedCode: InvalidCommandArguments,
		BasicCode:    550,
		Class:        ClassPermanentFailure,
		Comment:      "Domain cannot exceed 255 characters",
	}).String()

	Canned.FailBackendNotRunning = (&Response{
		EnhancedCode: OtherOrUndefinedProtocolStatus,
		BasicCode:    554,
		Class:        ClassPermanentFailure,
		Comment:      "Transaction failed - backend not running ",
	}).String()

	Canned.FailBackendTransaction = (&Response{
		EnhancedCode: OtherOrUndefinedProtocolStatus,
		BasicCode:    554,
		Class:        ClassPermanentFailure,
		Comment:      "Error: ",
	}).String()

	Canned.SuccessMessageQueued = (&Response{
		EnhancedCode: OtherStatus,
		BasicCode:    250,
		Class:        ClassSuccess,
		Comment:      "OK : queued as ",
	}).String()

	Canned.FailBackendTimeout = (&Response{
		EnhancedCode: OtherOrUndefinedProtocolStatus,
		BasicCode:    554,
		Class:        ClassPermanentFailure,
		Comment:      "Error: transaction timeout",
	}).String()

}

// DefaultMap contains defined default codes (RfC 3463)
const (
	OtherStatus                             = ".0.0"
	OtherAddressStatus                      = ".1.0"
	BadDestinationMailboxAddress            = ".1.1"
	BadDestinationSystemAddress             = ".1.2"
	BadDestinationMailboxAddressSyntax      = ".1.3"
	DestinationMailboxAddressAmbiguous      = ".1.4"
	DestinationMailboxAddressValid          = ".1.5"
	MailboxHasMoved                         = ".1.6"
	BadSendersMailboxAddressSyntax          = ".1.7"
	BadSendersSystemAddress                 = ".1.8"
	OtherOrUndefinedMailboxStatus           = ".2.0"
	MailboxDisabled                         = ".2.1"
	MailboxFull                             = ".2.2"
	MessageLengthExceedsAdministrativeLimit = ".2.3"
	MailingListExpansionProblem             = ".2.4"
	OtherOrUndefinedMailSystemStatus        = ".3.0"
	MailSystemFull                          = ".3.1"
	SystemNotAcceptingNetworkMessages       = ".3.2"
	SystemNotCapableOfSelectedFeatures      = ".3.3"
	MessageTooBigForSystem                  = ".3.4"
	OtherOrUndefinedNetworkOrRoutingStatus  = ".4.0"
	NoAnswerFromHost                        = ".4.1"
	BadConnection                           = ".4.2"
	RoutingServerFailure                    = ".4.3"
	UnableToRoute                           = ".4.4"
	NetworkCongestion                       = ".4.5"
	RoutingLoopDetected                     = ".4.6"
	DeliveryTimeExpired                     = ".4.7"
	OtherOrUndefinedProtocolStatus          = ".5.0"
	InvalidCommand                          = ".5.1"
	SyntaxError                             = ".5.2"
	TooManyRecipients                       = ".5.3"
	InvalidCommandArguments                 = ".5.4"
	WrongProtocolVersion                    = ".5.5"
	OtherOrUndefinedMediaError              = ".6.0"
	MediaNotSupported                       = ".6.1"
	ConversionRequiredAndProhibited         = ".6.2"
	ConversionRequiredButNotSupported       = ".6.3"
	ConversionWithLossPerformed             = ".6.4"
	ConversionFailed                        = ".6.5"
)

// TODO: More defaults needed....
var defaultTexts = struct {
	m map[string]string
}{m: map[string]string{
	"2.0.0": "OK",
	"2.1.0": "OK",
	"2.1.5": "Recipient valid",
	"2.5.0": "OK",
	"4.5.3": "Too many recipients",
	"4.5.4": "Relay access denied",
	"5.5.1": "Invalid command",
}}

// Response type for Stringer interface
type Response struct {
	EnhancedCode string
	BasicCode    int
	Class        class
	// Comment is optional
	Comment string
}

// EnhancedStatus are the ones that look like 2.1.0
type EnhancedStatus struct {
	Class  class
	Status string
}

// String returns a string representation of EnhancedStatus
func (e EnhancedStatus) String() string {
	return fmt.Sprintf("%d%s", e.Class, e.Status)
}

// String returns a custom Response as a string
func (r *Response) String() string {

	basicCode := r.BasicCode
	comment := r.Comment
	if len(comment) == 0 && r.BasicCode == 0 {
		comment = defaultTexts.m[r.EnhancedCode]
		if len(comment) == 0 {
			switch r.Class {
			case 2:
				comment = "OK"
			case 4:
				comment = "Temporary failure."
			case 5:
				comment = "Permanent failure."
			}
		}
	}
	e := EnhancedStatus{r.Class, r.EnhancedCode}
	if r.BasicCode == 0 {
		basicCode = getBasicStatusCode(e)
	}

	return fmt.Sprintf("%d %s %s", basicCode, e.String(), comment)
}

// getBasicStatusCode gets the basic status code from codeMap, or fallback code if not mapped
func getBasicStatusCode(e EnhancedStatus) int {
	if val, ok := codeMap.m[e]; ok {
		return val
	}
	// Fallback if code is not defined
	return int(e.Class) * 100
}
