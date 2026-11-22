package coach

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	SafetyGeneral = "general"
	SafetyCaution = "caution"
	SafetyUrgent  = "urgent"
)

type safetyAssessment struct {
	Classification string
	Hypothetical   bool
	Historical     bool
}

var (
	coachSymptomPattern = regexp.MustCompile(`(?i)\b(?:pain(?:ful)?|hurt(?:s|ing)?|sore(?:ness)?|discomfort|uncomfortable|ache(?:s|d|ing)?|injur(?:y|ies|ed)|pinch(?:es|ed|ing)?|strain(?:s|ed|ing)?|stiff(?:ness)?)\b`)
	coachUrgentPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:chest\s+(?:pain|pressure|tightness)|(?:pain|pressure|tightness)\s+in\s+(?:my|the|your|their)\s+chest|my\s+chest\s+(?:hurts?|feels?\s+(?:tight|painful|pressured)))\b`),
		regexp.MustCompile(`(?i)\b(?:shortness\s+of\s+breath|difficulty\s+breathing|trouble\s+breathing|(?:cannot|can't|unable\s+to)\s+breathe|gasping\s+for\s+air)\b`),
		regexp.MustCompile(`(?i)\b(?:fainted|fainting|passed\s+out|lost\s+consciousness|blacked\s+out|blackout|feel\s+faint|feeling\s+faint)\b`),
		regexp.MustCompile(`(?i)\b(?:severe|excruciating|unbearable)\s+(?:chest\s+)?pain\b`),
		regexp.MustCompile(`(?i)\b(?:sudden\s+)?(?:one[- ]sided\s+weakness|weakness\s+on\s+(?:one|my|your|their)\s+side|weakness\s+on\s+(?:the\s+)?(?:left|right)\s+side|loss\s+of\s+strength\s+on\s+one\s+side|one\s+side\s+of\s+my\s+body\s+is\s+weak|suddenly\s+(?:my\s+)?(?:left|right)\s+side\s+(?:feels?\s+)?weak|sudden\s+numbness|face\s+drooping|slurred\s+speech|sudden\s+confusion)\b`),
		regexp.MustCompile(`(?i)\b(?:cannot|can't|unable\s+to)\s+(?:move\s+(?:my|the)\s+(?:arm|leg|hand|foot)|use\s+(?:my|the)\s+(?:arm|leg|hand|foot)|bear\s+weight)|\b(?:loss|lost)\s+of\s+function\b`),
	}
	coachHistoricalPattern   = regexp.MustCompile(`(?i)\b(?:\d+\s+(?:hours?|days?|weeks?|months?)\s+ago|yesterday|last\s+(?:night|week|month|year|workout)|earlier|previously|in\s+the\s+past|used\s+to|resolved|went\s+away|no\s+longer|has\s+stopped|stopped)\b`)
	coachHypotheticalPattern = regexp.MustCompile(`(?i)\b(?:what\s+if|what\s+should\s+(?:i|we|someone|anyone|they)\s+do\s+if|if\s+(?:someone|anyone|a\s+person|somebody)|hypothetically|suppose\s+(?:someone|a\s+person)|in\s+general)\b`)
	coachClauseBoundary      = regexp.MustCompile(`[,;.!?\n]|\b(?:but|however|although|yet|except)\b`)
	coachNegationWords       = map[string]struct{}{
		"no": {}, "not": {}, "never": {}, "without": {}, "don't": {}, "doesn't": {}, "didn't": {},
		"can't": {}, "cannot": {}, "isn't": {}, "aren't": {}, "wasn't": {}, "weren't": {},
	}
)

func classifyRequestSafety(request ChatRequest) safetyAssessment {
	messages := []string{request.Message}
	for _, turn := range request.PreviousTurns {
		if turn.Role == "user" {
			messages = append(messages, turn.Content)
		}
	}

	result := safetyAssessment{Classification: SafetyGeneral}
	caution := false
	actualConcern := false
	for _, message := range messages {
		assessment := classifySafety(message)
		if assessment.Classification == SafetyUrgent {
			return assessment
		}
		if assessment.Classification == SafetyCaution {
			caution = true
			result.Hypothetical = result.Hypothetical || assessment.Hypothetical
			result.Historical = result.Historical || assessment.Historical
			if !assessment.Hypothetical && !assessment.Historical {
				actualConcern = true
			}
		}
	}
	if caution {
		result.Classification = SafetyCaution
		if actualConcern {
			result.Hypothetical = false
			result.Historical = false
		}
	}
	return result
}

func classifySafety(message string) safetyAssessment {
	assessment := safetyAssessment{Classification: SafetyGeneral}
	concern, actualConcern := false, false
	text := strings.NewReplacer("\u2019", "'", "\u2018", "'").Replace(strings.ToLower(message))
	quoted := quotedRanges(message)
	quoteFree := strings.NewReplacer("\u2019", "'", "\u2018", "'").Replace(strings.ToLower(withoutQuotedRanges(message, quoted)))

	for _, pattern := range append([]*regexp.Regexp{coachSymptomPattern}, coachUrgentPatterns...) {
		for _, match := range pattern.FindAllStringIndex(text, -1) {
			if occurrenceNegated(text, match) {
				continue
			}
			concern = true
			clause := clauseFor(text, match)
			hypothetical := coachHypotheticalPattern.MatchString(clause)
			historical := coachHistoricalPattern.MatchString(clause)
			if hypothetical {
				assessment.Hypothetical = true
			} else if historical {
				assessment.Historical = true
			} else if !isInsideQuotedRange(match[0], quoted) {
				actualConcern = true
			} else {
				assessment.Hypothetical = true
			}
		}
	}
	if !concern {
		return assessment
	}

	for _, pattern := range coachUrgentPatterns {
		for _, match := range pattern.FindAllStringIndex(quoteFree, -1) {
			if occurrenceNegated(quoteFree, match) {
				continue
			}
			clause := clauseFor(quoteFree, match)
			if coachHistoricalPattern.MatchString(clause) || coachHypotheticalPattern.MatchString(clause) {
				continue
			}
			assessment.Classification = SafetyUrgent
			return assessment
		}
	}

	assessment.Classification = SafetyCaution
	if actualConcern {
		assessment.Hypothetical = false
		assessment.Historical = false
	}
	return assessment
}

type quotedRange struct{ start, end int }

func quotedRanges(message string) []quotedRange {
	runes := []rune(message)
	ranges := make([]quotedRange, 0, 2)
	var closeQuote rune
	start := -1
	byteOffset := 0
	for i, r := range runes {
		size := len(string(r))
		if closeQuote != 0 {
			if r == closeQuote {
				ranges = append(ranges, quotedRange{start: start, end: byteOffset + size})
				closeQuote = 0
				start = -1
			}
		} else {
			switch r {
			case '"':
				closeQuote, start = '"', byteOffset
			case '\'':
				if !isApostrophe(runes, i) {
					closeQuote, start = '\'', byteOffset
				}
			case '\u201c':
				closeQuote, start = '\u201d', byteOffset
			case '\u2018':
				closeQuote, start = '\u2019', byteOffset
			}
		}
		byteOffset += size
	}
	if closeQuote != 0 {
		ranges = append(ranges, quotedRange{start: start, end: len(message)})
	}
	return ranges
}

func isApostrophe(runes []rune, i int) bool {
	return i > 0 && i+1 < len(runes) && unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1])
}

func withoutQuotedRanges(message string, ranges []quotedRange) string {
	if len(ranges) == 0 {
		return message
	}
	text := []byte(message)
	for _, quoted := range ranges {
		for i := quoted.start; i < quoted.end && i < len(text); i++ {
			text[i] = ' '
		}
	}
	return string(text)
}

func isInsideQuotedRange(offset int, ranges []quotedRange) bool {
	for _, quoted := range ranges {
		if offset >= quoted.start && offset < quoted.end {
			return true
		}
	}
	return false
}

func clauseFor(text string, match []int) string {
	start, end := 0, len(text)
	for _, boundary := range coachClauseBoundary.FindAllStringIndex(text[:match[0]], -1) {
		start = boundary[1]
	}
	for _, boundary := range coachClauseBoundary.FindAllStringIndex(text[match[1]:], -1) {
		end = match[1] + boundary[0]
		break
	}
	return text[start:end]
}

func occurrenceNegated(text string, match []int) bool {
	start := 0
	for _, boundary := range coachClauseBoundary.FindAllStringIndex(text[:match[0]], -1) {
		start = boundary[1]
	}
	words := strings.Fields(text[start:match[0]])
	for i, word := range words {
		word = strings.Trim(word, `"'()[]{}:`)
		if _, ok := coachNegationWords[word]; ok && len(words)-i <= 5 {
			return true
		}
	}
	return false
}

func cautionAdvisory(assessment safetyAssessment) string {
	if assessment.Hypothetical {
		return "Safety: If someone is experiencing these symptoms now, they should stop activity and seek appropriate medical help. Severe chest pain, difficulty breathing, fainting, or sudden one-sided weakness warrant urgent or emergency care."
	}
	if assessment.Historical {
		return "Safety: Since you describe this as past discomfort, avoid the movement if symptoms return. Seek professional evaluation if symptoms persist, worsen, recur, or affect normal activities."
	}
	return "Safety: Stop the movement that causes pain; do not train through pain. Seek professional evaluation if symptoms persist, worsen, recur, or affect normal activities."
}

func cautionActions(assessment safetyAssessment) []string {
	if assessment.Hypothetical {
		return []string{"stop_activity_if_symptoms_are_present", "seek_emergency_help_for_red_flags"}
	}
	if assessment.Historical {
		return []string{"avoid_triggering_movement_if_symptoms_return", "seek_professional_guidance_if_recurrent"}
	}
	return []string{"stop_painful_movement", "seek_professional_guidance_if_persistent"}
}

func cautionFallback(requestID string, exercise *ExerciseReference, assessment safetyAssessment) ChatResponse {
	answer := "I cannot determine the cause of discomfort from text. " + cautionAdvisory(assessment)
	return ChatResponse{
		Answer: answer, Exercise: exercise, SafetyClassification: SafetyCaution, SuggestedActions: cautionActions(assessment),
		RequestID: requestID, Limitations: "General safety information only; this is not a diagnosis or medical clearance.",
	}
}
