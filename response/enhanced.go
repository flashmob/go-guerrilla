package response

import (
	"fmt"
	"strconv"
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

// codeMap for mapping Enhanced Status Code to Basic Code
// Mapping according to https://www.iana.org/assignments/smtp-enhanced-status-codes/smtp-enhanced-status-codes.xml
// This might not be entierly useful
var codeMap = struct {
	m map[string]int
}{m: map[string]int{
	"2.1.0":  250,
	"2.1.5":  250,
	"2.3.0":  250,
	"2.5.0":  250,
	"2.6.4":  250,
	"2.6.8":  252,
	"2.7.0":  220,
	"4.1.1":  451,
	"4.1.8":  451,
	"4.2.4":  450,
	"4.3.0":  421,
	"4.3.1":  452,
	"4.3.2":  453,
	"4.4.1":  451,
	"4.4.2":  421,
	"4.4.3":  451,
	"4.4.5":  451,
	"4.5.0":  451,
	"4.5.1":  430,
	"4.5.3":  452,
	"4.5.4":  451,
	"4.7.0":  450,
	"4.7.1":  451,
	"4.7.12": 422,
	"4.7.15": 450,
	"4.7.24": 451,
	"5.1.1":  550,
	"5.1.3":  501,
	"5.1.8":  501,
	"5.1.10": 556,
	"5.2.2":  552,
	"5.2.3":  552,
	"5.3.0":  550,
	"5.3.4":  552,
	"5.4.3":  550,
	"5.5.0":  501,
	"5.5.1":  500,
	"5.5.2":  500,
	"5.5.4":  501,
	"5.5.6":  500,
	"5.6.3":  554,
	"5.6.6":  554,
	"5.6.7":  553,
	"5.6.8":  550,
	"5.6.9":  550,
	"5.7.0":  550,
	"5.7.1":  551,
	"5.7.2":  550,
	"5.7.4":  504,
	"5.7.8":  554,
	"5.7.9":  534,
	"5.7.10": 523,
	"5.7.11": 524,
	"5.7.13": 525,
	"5.7.14": 535,
	"5.7.15": 550,
	"5.7.16": 552,
	"5.7.17": 500,
	"5.7.18": 500,
	"5.7.19": 500,
	"5.7.20": 550,
	"5.7.21": 550,
	"5.7.22": 550,
	"5.7.23": 550,
	"5.7.24": 550,
	"5.7.25": 550,
	"5.7.26": 550,
	"5.7.27": 550,
}}

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
	Class        int
	// Comment is optional
	Comment string
}

// Custom returns a custom Response Stringer
func (r *Response) String() string {
	e := buildEnhancedResponseFromDefaultStatus(r.Class, r.EnhancedCode)
	basicCode := r.BasicCode
	comment := r.Comment
	if len(comment) == 0 {
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
	if r.BasicCode == 0 {
		basicCode = getBasicStatusCode(e)
	}

	return fmt.Sprintf("%d %s %s", basicCode, e, comment)
}

/*
// CustomString builds an enhanced status code string using your custom string and basic code
func CustomString(enhancedCode string, basicCode, class int, comment string) string {
	e := buildEnhancedResponseFromDefaultStatus(class, enhancedCode)
	return fmt.Sprintf("%d %s %s", basicCode, e, comment)
}

// String builds an enhanced status code string
func String(enhancedCode string, class int) string {
	e := buildEnhancedResponseFromDefaultStatus(class, enhancedCode)
	basicCode := getBasicStatusCode(e)
	comment := defaultTexts.m[enhancedCode]

	if len(comment) == 0 {
		switch class {
		case 2:
			comment = "OK"
		case 4:
			comment = "Temporary failure."
		case 5:
			comment = "Permanent failure."
		}
	}
	return CustomString(enhancedCode, basicCode, class, comment)
}
*/
func getBasicStatusCode(enhancedStatusCode string) int {
	if val, ok := codeMap.m[enhancedStatusCode]; ok {
		return val
	}
	// Fallback if code is not defined
	fb, _ := strconv.Atoi(fmt.Sprintf("%c00", enhancedStatusCode[0]))
	return fb
}

func buildEnhancedResponseFromDefaultStatus(c int, status string) string {
	// Construct code
	return fmt.Sprintf("%d%s", c, status)
}
