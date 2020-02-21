package attestations

import (
	"context"
	"fmt"

	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	"github.com/prysmaticlabs/prysm/shared/params"
)

// DetectionKind defines an enum type that
// gives us information on the type of slashable offense
// found when analyzing validator min-max spans.
type DetectionKind int

const (
	NotFound DetectionKind = iota
	DoubleVote
	SurroundVote
)

// DetectionResult tells us the kind of slashable
// offense found from detecting on min-max spans +
// the slashable epoch for the offense.
type DetectionResult struct {
	Kind           DetectionKind
	SlashableEpoch uint64
}

// SpanDetector defines a struct which can detect slashable
// attestation offenses by tracking validator min-max
// spans from validators.
type SpanDetector struct {
	// Slice of epochs for valindex => min-max span.
	spans []map[uint64][2]uint16
}

// NewSpanDetector --
func NewSpanDetector() *SpanDetector {
	return &SpanDetector{
		spans: make([]map[uint64][2]uint16, 256),
	}
}

// DetectSlashingForValidator --
func (s *SpanDetector) DetectSlashingForValidator(
	ctx context.Context,
	validatorIdx uint64,
	sourceEpoch uint64,
	targetEpoch uint64,
) (*DetectionResult, error) {
	if (targetEpoch - sourceEpoch) > params.BeaconConfig().WeakSubjectivityPeriod {
		return nil, fmt.Errorf(
			"attestation span was greater than weak subjectivity period %d, received: %d",
			params.BeaconConfig().WeakSubjectivityPeriod,
			targetEpoch-sourceEpoch,
		)
	}
	distance := uint16(targetEpoch - sourceEpoch)
	numSpans := uint64(len(s.spans))
	if val := s.spans[sourceEpoch%numSpans]; val != nil {
		minSpan := val[validatorIdx][0]
		if minSpan > 0 && minSpan < distance {
			return &DetectionResult{
				Kind:           SurroundVote,
				SlashableEpoch: uint64(minSpan) + sourceEpoch,
			}, nil
		}

		maxSpan := val[validatorIdx][1]
		if maxSpan > distance {
			return &DetectionResult{
				Kind:           SurroundVote,
				SlashableEpoch: uint64(maxSpan) + sourceEpoch,
			}, nil
		}
	}
	return nil, nil
}

// SpansForValidatorByEpoch --
func (s *SpanDetector) SpansForValidatorByEpoch(ctx context.Context, valIdx uint64, epoch uint64) ([2]uint16, error) {
	numSpans := uint64(len(s.spans))
	if span := s.spans[epoch%numSpans]; span != nil {
		if minMaxSpan, ok := span[valIdx]; ok {
			return minMaxSpan, nil
		}
		return [2]uint16{}, fmt.Errorf("validator index %d not found in span map", valIdx)
	}
	return [2]uint16{}, fmt.Errorf("no span found for epoch %d", epoch)
}

//  ValidatorSpansByEpoch --
func (s *SpanDetector) ValidatorSpansByEpoch(ctx context.Context, epoch uint64) map[uint64][2]uint16 {
	numSpans := uint64(len(s.spans))
	return s.spans[epoch%numSpans]
}

//  DeleteValidatorSpansByEpoch --
func (s *SpanDetector) DeleteValidatorSpansByEpoch(ctx context.Context, validatorIdx uint64, epoch uint64) error {
	numSpans := uint64(len(s.spans))
	if val := s.spans[epoch%numSpans]; val != nil {
		delete(val, validatorIdx)
	}
	return fmt.Errorf("no span map found at epoch %d", epoch)
}

// UpdateSpans given an indexed attestation for all of its attesting indices.
func (s *SpanDetector) UpdateSpans(ctx context.Context, att *ethpb.IndexedAttestation) error {
	source := att.Data.Source.Epoch
	target := att.Data.Target.Epoch
	// Update spansForEpoch[valIdx] using the source/target data for
	// each validator in attesting indices.
	for i := 0; i < len(att.AttestingIndices); i++ {
		valIdx := att.AttestingIndices[i]
		// Update min and max spans.
		s.updateMinSpan(source, target, valIdx)
		s.updateMaxSpan(source, target, valIdx)
	}
	return nil
}

func (s *SpanDetector) updateMinSpan(source uint64, target uint64, valIdx uint64) {
	numSpans := uint64(len(s.spans))
	for epoch := source - 1; epoch > 0; epoch-- {
		val := uint16(target - (epoch))
		if sp := s.spans[epoch%numSpans]; sp == nil {
			s.spans[epoch%numSpans] = make(map[uint64][2]uint16)
		}
		minSpan := s.spans[epoch%numSpans][valIdx][0]
		maxSpan := s.spans[epoch%numSpans][valIdx][1]
		if minSpan == 0 || minSpan > val {
			s.spans[epoch%numSpans][valIdx] = [2]uint16{val, maxSpan}
		} else {
			break
		}
	}
}

func (s *SpanDetector) updateMaxSpan(source uint64, target uint64, valIdx uint64) {
	numSpans := uint64(len(s.spans))
	distance := target - source
	for epoch := uint64(1); epoch < distance; epoch++ {
		val := uint16(distance - epoch)
		if sp := s.spans[source+epoch%numSpans]; sp == nil {
			s.spans[source+epoch%numSpans] = make(map[uint64][2]uint16)
		}
		minSpan := s.spans[source+epoch%numSpans][valIdx][0]
		maxSpan := s.spans[source+epoch%numSpans][valIdx][1]
		if maxSpan < val {
			s.spans[source+epoch%numSpans][valIdx] = [2]uint16{minSpan, val}
		} else {
			break
		}
	}
}
