package bpmn

import (
	"fmt"
	"sort"
)

// Severity adalah tingkat keparahan issue validasi.
type Severity uint8

const (
	// SeverityError membuat resource ditolak.
	SeverityError Severity = iota + 1
	// SeverityWarning hanya informasi; resource tetap diterima.
	SeverityWarning
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Code adalah kode issue yang stabil untuk UI dan test.
type Code string

// Kode fatal (dikembalikan Parse sebagai *ParseError).
const (
	CodeXMLMalformed     Code = "XML_MALFORMED"
	CodeNotBPMN          Code = "NOT_BPMN"
	CodeResourceTooLarge Code = "RESOURCE_TOO_LARGE"
	CodeTooManyElements  Code = "TOO_MANY_ELEMENTS"
	CodeNestingTooDeep   Code = "NESTING_TOO_DEEP"
)

// Kode validasi (dikembalikan Validate).
const (
	CodeNoExecutableProcess          Code = "NO_EXECUTABLE_PROCESS"
	CodeProcessNotExecutable         Code = "PROCESS_NOT_EXECUTABLE"
	CodeMissingID                    Code = "MISSING_ID"
	CodeInvalidID                    Code = "INVALID_ID"
	CodeDuplicateID                  Code = "DUPLICATE_ID"
	CodeDuplicateProcessID           Code = "DUPLICATE_PROCESS_ID"
	CodeUnsupportedElement           Code = "UNSUPPORTED_ELEMENT"
	CodeUnsupportedEventDefinition   Code = "UNSUPPORTED_EVENT_DEFINITION"
	CodeMultipleEventDefinitions     Code = "MULTIPLE_EVENT_DEFINITIONS"
	CodeNoStartEvent                 Code = "NO_START_EVENT"
	CodeMultipleNoneStartEvents      Code = "MULTIPLE_NONE_START_EVENTS"
	CodeSubprocessInvalidStartEvent  Code = "SUBPROCESS_INVALID_START_EVENT"
	CodeStartEventHasIncoming        Code = "START_EVENT_HAS_INCOMING"
	CodeBoundaryEventHasIncoming     Code = "BOUNDARY_EVENT_HAS_INCOMING"
	CodeEndEventHasOutgoing          Code = "END_EVENT_HAS_OUTGOING"
	CodeFlowSourceNotFound           Code = "FLOW_SOURCE_NOT_FOUND"
	CodeFlowTargetNotFound           Code = "FLOW_TARGET_NOT_FOUND"
	CodeFlowCrossesScope             Code = "FLOW_CROSSES_SCOPE"
	CodeFlowRefInconsistent          Code = "FLOW_REF_INCONSISTENT"
	CodeElementUnreachable           Code = "ELEMENT_UNREACHABLE"
	CodeImplicitEnd                  Code = "IMPLICIT_END"
	CodeTaskDefinitionMissing        Code = "TASK_DEFINITION_MISSING"
	CodeTaskDefinitionTypeEmpty      Code = "TASK_DEFINITION_TYPE_EMPTY"
	CodeTaskDefinitionRetriesInvalid Code = "TASK_DEFINITION_RETRIES_INVALID"
	CodeTaskHeaderDuplicateKey       Code = "TASK_HEADER_DUPLICATE_KEY"
	CodeIOMappingTargetEmpty         Code = "IO_MAPPING_TARGET_EMPTY"
	CodeIOMappingSourceEmpty         Code = "IO_MAPPING_SOURCE_EMPTY"
	CodeExpressionEmpty              Code = "EXPRESSION_EMPTY"
	CodeGatewayDefaultFlowInvalid    Code = "GATEWAY_DEFAULT_FLOW_INVALID"
	CodeDefaultFlowHasCondition      Code = "DEFAULT_FLOW_HAS_CONDITION"
	CodeGatewayFlowMissingCondition  Code = "GATEWAY_FLOW_MISSING_CONDITION"
	CodeParallelGatewayFlowCondition Code = "PARALLEL_GATEWAY_FLOW_HAS_CONDITION"
	CodeConditionOnNonGatewayFlow    Code = "CONDITION_ON_NON_GATEWAY_FLOW"
	CodeEventGatewayInvalidTarget    Code = "EVENT_GATEWAY_INVALID_TARGET"
	CodeTimerDefinitionInvalid       Code = "TIMER_DEFINITION_INVALID"
	CodeTimerCycleNotAllowed         Code = "TIMER_CYCLE_NOT_ALLOWED"
	CodeMessageRefNotFound           Code = "MESSAGE_REF_NOT_FOUND"
	CodeMessageNameMissing           Code = "MESSAGE_NAME_MISSING"
	CodeMessageCorrelationKeyMissing Code = "MESSAGE_CORRELATION_KEY_MISSING"
	CodeErrorRefNotFound             Code = "ERROR_REF_NOT_FOUND"
	CodeErrorCodeMissing             Code = "ERROR_CODE_MISSING"
	CodeEscalationRefNotFound        Code = "ESCALATION_REF_NOT_FOUND"
	CodeEscalationCodeMissing        Code = "ESCALATION_CODE_MISSING"
	CodeSignalRefNotFound            Code = "SIGNAL_REF_NOT_FOUND"
	CodeSignalNameMissing            Code = "SIGNAL_NAME_MISSING"
	CodeBoundaryAttachedToNotFound   Code = "BOUNDARY_ATTACHED_TO_NOT_FOUND"
	CodeBoundaryInvalidHost          Code = "BOUNDARY_INVALID_HOST"
	CodeBoundaryDuplicateErrorCatch  Code = "BOUNDARY_DUPLICATE_ERROR_CATCH"
	CodeCalledElementMissing         Code = "CALLED_ELEMENT_MISSING"
	CodeMultiInstanceInputMissing    Code = "MULTI_INSTANCE_INPUT_COLLECTION_MISSING"
	CodeMultiInstanceOutputPartial   Code = "MULTI_INSTANCE_OUTPUT_INCOMPLETE"
	CodeUserTaskFormConflict         Code = "USER_TASK_FORM_CONFLICT"
	CodeEventSubprocessInvalidStart  Code = "EVENT_SUBPROCESS_INVALID_START"
	CodeExecutionPlatformMismatch    Code = "EXECUTION_PLATFORM_MISMATCH"
	CodeCamunda7ExtensionIgnored     Code = "CAMUNDA7_EXTENSION_IGNORED"
)

// Position adalah lokasi elemen di XML sumber (1-based).
type Position struct {
	Line   int
	Column int
}

// Issue adalah satu temuan validasi.
type Issue struct {
	Code      Code
	Severity  Severity
	ProcessID string
	ElementID string
	Message   string
	Position  Position
}

func (i Issue) String() string {
	return fmt.Sprintf("%d:%d %s %s [%s] %s", i.Position.Line, i.Position.Column, i.Severity, i.Code, i.ElementID, i.Message)
}

// Issues adalah kumpulan issue terurut stabil.
type Issues []Issue

// HasErrors mengembalikan true jika ada issue dengan severity error.
func (is Issues) HasErrors() bool {
	for _, i := range is {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Errors mengembalikan issue berseverity error saja.
func (is Issues) Errors() Issues { return is.filter(SeverityError) }

// Warnings mengembalikan issue berseverity warning saja.
func (is Issues) Warnings() Issues { return is.filter(SeverityWarning) }

func (is Issues) filter(s Severity) Issues {
	var out Issues
	for _, i := range is {
		if i.Severity == s {
			out = append(out, i)
		}
	}
	return out
}

func (is Issues) sort() {
	sort.SliceStable(is, func(a, b int) bool {
		x, y := is[a], is[b]
		if x.Position.Line != y.Position.Line {
			return x.Position.Line < y.Position.Line
		}
		if x.Position.Column != y.Position.Column {
			return x.Position.Column < y.Position.Column
		}
		if x.Code != y.Code {
			return x.Code < y.Code
		}
		return x.ElementID < y.ElementID
	})
}

// ParseError adalah kegagalan fatal Parse: XML rusak, bukan BPMN, atau melewati batas.
type ParseError struct {
	Code     Code
	Message  string
	Position Position
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s at %d:%d: %s", e.Code, e.Position.Line, e.Position.Column, e.Message)
}

// Issue mengubah ParseError menjadi Issue berseverity error.
func (e *ParseError) Issue() Issue {
	return Issue{Code: e.Code, Severity: SeverityError, Message: e.Message, Position: e.Position}
}
