package eval

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/piovani/RAGscope/internal/domain"
	"github.com/piovani/RAGscope/internal/generate"
	"github.com/piovani/RAGscope/internal/retrieve"
)

// citation matches the [1], [2]... that the prompt asks the model to write.
var citation = regexp.MustCompile(`\[\d+\]`)

// Judge tells whether an answer is supported by the passages it was built from.
type Judge interface {
	Supported(ctx context.Context, question string, passages []domain.ScoredChunk, answer string) (bool, error)
}

// AnswerResult is what the model did for one question.
type AnswerResult struct {
	ID           string   `json:"id"`
	Question     string   `json:"question"`
	Unanswerable bool     `json:"unanswerable"`
	Answer       string   `json:"answer"`
	Refused      bool     `json:"refused"`
	Cited        bool     `json:"cited"`
	Supported    *bool    `json:"supported,omitempty"` // set only when a judge ran
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	LatencyMS    float64  `json:"latency_ms"`
	Retrieved    []string `json:"retrieved"`
}

// AnswerReport is the outcome of generating answers for the golden set.
type AnswerReport struct {
	Experiment string    `json:"experiment"`
	Generator  string    `json:"generator"`
	Retriever  string    `json:"retriever"`
	K          int       `json:"k"`
	RunAt      time.Time `json:"run_at"`
	GoldenSet  string    `json:"golden_set,omitempty"`

	Answerable   int `json:"answerable"`   // questions the law answers
	Unanswerable int `json:"unanswerable"` // questions the law does not answer

	// CorrectRefusal is the share of unanswerable questions the model refused.
	// FalseRefusal is the share of answerable questions it refused by mistake.
	CorrectRefusal float64 `json:"correct_refusal_rate"`
	FalseRefusal   float64 `json:"false_refusal_rate"`
	// Citation is the share of real answers that cite at least one passage.
	Citation float64 `json:"citation_rate"`
	// Faithfulness is the share of judged answers the judge found supported by
	// the passages. Zero Judged means no judge ran.
	Faithfulness float64 `json:"faithfulness,omitempty"`
	Judged       int     `json:"judged,omitempty"`

	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	LatencyMean  float64 `json:"latency_mean_ms"`
	LatencyP95   float64 `json:"latency_p95_ms"`

	Results []AnswerResult `json:"results"`
}

// RunAnswers retrieves passages for every question, asks the model to answer
// from them and scores the behaviour. The judge is optional (nil to skip it).
func RunAnswers(
	ctx context.Context,
	retriever retrieve.Retriever,
	generator generate.Generator,
	judge Judge,
	questions []Question,
	k int,
) (*AnswerReport, error) {
	report := &AnswerReport{Generator: generator.Name(), Retriever: retriever.Name(), K: k, RunAt: time.Now()}

	for _, q := range questions {
		passages, err := retriever.Retrieve(ctx, q.Question, k)
		if err != nil {
			return nil, fmt.Errorf("question %s: retrieve: %w", q.ID, err)
		}

		resp, err := generator.Generate(ctx, generate.Request{Question: q.Question, Context: passages})
		if err != nil {
			return nil, fmt.Errorf("question %s: %w", q.ID, err)
		}

		ids := make([]string, len(passages))
		for i, p := range passages {
			ids[i] = p.ID
		}
		result := AnswerResult{
			ID:           q.ID,
			Question:     q.Question,
			Unanswerable: q.Unanswerable,
			Answer:       resp.Answer,
			Refused:      resp.Refused,
			Cited:        citation.MatchString(resp.Answer),
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			LatencyMS:    float64(resp.Usage.Latency) / float64(time.Millisecond),
			Retrieved:    ids,
		}

		// Only a real answer can be unfaithful; a refusal makes no claims.
		if judge != nil && !resp.Refused {
			supported, err := judge.Supported(ctx, q.Question, passages, resp.Answer)
			if err != nil {
				return nil, fmt.Errorf("question %s: judge: %w", q.ID, err)
			}
			result.Supported = &supported
		}
		report.Results = append(report.Results, result)
	}

	if len(report.Results) == 0 {
		return nil, fmt.Errorf("there are no questions to answer")
	}
	report.summarize()
	return report, nil
}

func (r *AnswerReport) summarize() {
	var correctRefusals, falseRefusals, realAnswers, cited, supported int
	latencies := make([]float64, 0, len(r.Results))

	for _, res := range r.Results {
		if res.Unanswerable {
			r.Unanswerable++
			if res.Refused {
				correctRefusals++
			}
		} else {
			r.Answerable++
			if res.Refused {
				falseRefusals++
			}
		}

		if !res.Refused {
			realAnswers++
			if res.Cited {
				cited++
			}
		}
		if res.Supported != nil {
			r.Judged++
			if *res.Supported {
				supported++
			}
		}

		r.InputTokens += res.InputTokens
		r.OutputTokens += res.OutputTokens
		r.LatencyMean += res.LatencyMS
		latencies = append(latencies, res.LatencyMS)
	}

	r.CorrectRefusal = share(correctRefusals, r.Unanswerable)
	r.FalseRefusal = share(falseRefusals, r.Answerable)
	r.Citation = share(cited, realAnswers)
	r.Faithfulness = share(supported, r.Judged)

	r.LatencyMean /= float64(len(r.Results))
	slices.Sort(latencies)
	r.LatencyP95 = latencies[(len(latencies)*95+99)/100-1]
}

func share(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total)
}

// Sample picks n questions spread evenly across the list, so a quick run still
// has answerable and unanswerable questions from every batch.
func Sample(questions []Question, n int) []Question {
	if n <= 0 || n >= len(questions) {
		return questions
	}
	picked := make([]Question, 0, n)
	for i := range n {
		picked = append(picked, questions[i*len(questions)/n])
	}
	return picked
}
