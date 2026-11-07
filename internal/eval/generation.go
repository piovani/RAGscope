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

// Judge grades answers. It is a model, so its grades must be checked against a person's.
type Judge interface {
	Supported(ctx context.Context, question string, passages []domain.ScoredChunk, answer string) (bool, error)
	// Correct tells whether the answer says what the reference answer says.
	Correct(ctx context.Context, question, reference, answer string) (bool, error)
}

// AnswerResult is what the model did for one question.
type AnswerResult struct {
	ID           string `json:"id"`
	Question     string `json:"question"`
	Unanswerable bool   `json:"unanswerable"`
	Answer       string `json:"answer"`
	Refused      bool   `json:"refused"`
	Cited        bool   `json:"cited"`
	Supported    *bool  `json:"supported,omitempty"` // faithfulness, set only when a judge ran
	Correct      *bool  `json:"correct,omitempty"`   // matches the reference answer, set only when a judge ran
	// EvidenceRetrieved tells whether the passages the model saw contained the
	// evidence. It separates a refusal caused by retrieval from a timid model.
	// Only set for answerable questions.
	EvidenceRetrieved *bool    `json:"evidence_retrieved,omitempty"`
	InputTokens       int      `json:"input_tokens"`
	OutputTokens      int      `json:"output_tokens"`
	CostUSD           float64  `json:"cost_usd,omitempty"`
	LatencyMS         float64  `json:"latency_ms"`
	Retrieved         []string `json:"retrieved"`
	// Passages are the texts the model saw, in order. A person needs them to
	// check whether an answer is supported by them.
	Passages []string `json:"passages,omitempty"`
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
	// A false refusal is the retriever's doing when the evidence was not among
	// the passages the model saw (refusing was right, given what it had) and the
	// model's doing when it was (the model was too timid). Both are shares of
	// the answerable questions, and they add up to FalseRefusal.
	FalseRefusalNoEvidence   float64 `json:"false_refusal_no_evidence_rate"`
	FalseRefusalWithEvidence float64 `json:"false_refusal_with_evidence_rate"`

	// Faithfulness is the share of judged answers the judge found supported by
	// the passages. Zero Judged means this judge did not run.
	Faithfulness float64 `json:"faithfulness,omitempty"`
	Judged       int     `json:"judged,omitempty"`

	// Correct is the share of answerable questions answered correctly, as judged
	// against the reference answer. A refusal counts as wrong, because the law
	// had the answer. Zero CorrectJudged means this judge did not run.
	Correct       float64 `json:"correct_rate,omitempty"`
	CorrectJudged int     `json:"correct_judged,omitempty"`

	CostUSD float64 `json:"cost_usd,omitempty"` // only hosted models cost money

	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	LatencyMean  float64 `json:"latency_mean_ms"`
	LatencyP95   float64 `json:"latency_p95_ms"`

	Results []AnswerResult `json:"results"`
}

// JudgeOptions says which checks the judge performs. Both are slow, and the
// faithfulness check is the slower one because it sends the passages too.
type JudgeOptions struct {
	Faithfulness bool // is every claim supported by the passages?
	Correctness  bool // does the answer say what the reference answer says?
}

// RunAnswers retrieves passages for every question, asks the model to answer
// from them and scores the behaviour. The judge is optional (nil to skip it).
func RunAnswers(
	ctx context.Context,
	retriever retrieve.Retriever,
	generator generate.Generator,
	judge Judge,
	opts JudgeOptions,
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

		result := newAnswerResult(q, passages, resp)

		if judge != nil {
			if err := judgeAnswer(ctx, judge, opts, q, passages, &result); err != nil {
				return nil, fmt.Errorf("question %s: judge: %w", q.ID, err)
			}
		}
		report.Results = append(report.Results, result)
	}

	if len(report.Results) == 0 {
		return nil, fmt.Errorf("there are no questions to answer")
	}
	report.summarize()
	return report, nil
}

func newAnswerResult(q Question, passages []domain.ScoredChunk, resp generate.Response) AnswerResult {
	ids := make([]string, len(passages))
	texts := make([]string, len(passages))
	for i, p := range passages {
		ids[i], texts[i] = p.ID, p.Text
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
		CostUSD:      resp.Usage.CostUSD,
		LatencyMS:    float64(resp.Usage.Latency) / float64(time.Millisecond),
		Retrieved:    ids,
		Passages:     texts,
	}

	// Did the model have what it needed? Only meaningful when the law has the answer.
	if !q.Unanswerable {
		found := !slices.Contains(findRanks(passages, q.Evidence), 0)
		result.EvidenceRetrieved = &found
	}
	return result
}

// judgeAnswer asks the judge about one answer. Only a real answer can be
// unfaithful or wrong in what it says: a refusal makes no claims, and a refusal
// of an answerable question is wrong without needing anybody to look at it.
func judgeAnswer(ctx context.Context, judge Judge, opts JudgeOptions, q Question, passages []domain.ScoredChunk, result *AnswerResult) error {
	if result.Refused {
		return nil
	}

	if opts.Faithfulness {
		supported, err := judge.Supported(ctx, q.Question, passages, result.Answer)
		if err != nil {
			return err
		}
		result.Supported = &supported
	}

	// An answer to an unanswerable question is wrong by definition: the model
	// should have refused. So only answerable questions are compared.
	if opts.Correctness && !q.Unanswerable {
		if q.Answer == "" {
			return fmt.Errorf("the golden set has no reference answer for %s", q.ID)
		}
		correct, err := judge.Correct(ctx, q.Question, q.Answer, result.Answer)
		if err != nil {
			return err
		}
		result.Correct = &correct
	}
	return nil
}

func (r *AnswerReport) summarize() {
	var correctRefusals, falseRefusalsNoEvidence, falseRefusalsWithEvidence int
	var realAnswers, cited, supported, correct int
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
				if res.EvidenceRetrieved != nil && *res.EvidenceRetrieved {
					falseRefusalsWithEvidence++
				} else {
					falseRefusalsNoEvidence++
				}
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
		if res.Correct != nil {
			r.CorrectJudged++
			if *res.Correct {
				correct++
			}
		}

		r.InputTokens += res.InputTokens
		r.OutputTokens += res.OutputTokens
		r.CostUSD += res.CostUSD
		r.LatencyMean += res.LatencyMS
		latencies = append(latencies, res.LatencyMS)
	}

	r.CorrectRefusal = share(correctRefusals, r.Unanswerable)
	r.FalseRefusalNoEvidence = share(falseRefusalsNoEvidence, r.Answerable)
	r.FalseRefusalWithEvidence = share(falseRefusalsWithEvidence, r.Answerable)
	r.FalseRefusal = r.FalseRefusalNoEvidence + r.FalseRefusalWithEvidence
	r.Citation = share(cited, realAnswers)
	r.Faithfulness = share(supported, r.Judged)
	if r.CorrectJudged > 0 {
		// Refusals were never judged and count as wrong, so the base is every
		// answerable question.
		r.Correct = share(correct, r.Answerable)
	}

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
