package stateprocessor

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/prysmaticlabs/prysm/v3/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/helpers"

	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/state/stategen"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v3/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v3/encoding/ssz/detect"
	"github.com/prysmaticlabs/prysm/v3/explorer/tracer"
	"github.com/prysmaticlabs/prysm/v3/math"
	"github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1/attestation"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type EpochData struct {
	Blocks                  []interfaces.BeaconBlock
	Validators              map[types.ValidatorIndex]*tracer.ValidatorEpochData
	State                   state.BeaconState
	NumActivatingValidators int
	NumExitingValidators    int
	ProposerAssignments     map[types.ValidatorIndex][]types.Slot
	CommitteeAssignments    map[types.ValidatorIndex]*helpers.CommitteeAssignmentContainer
	PreviousEpochVotedGWei  uint64
	PreviousEpochActiveGWei uint64
}

func GetEpochData(s state.BeaconState, network string, epoch uint64, clClient *beacon.Client) (*EpochData, error) {

	data := &EpochData{
		Blocks:     make([]interfaces.BeaconBlock, 32),
		Validators: make(map[types.ValidatorIndex]*tracer.ValidatorEpochData),
	}

	tracer.ClearData()

	ctx := context.Background()

	if network == "sepolia" {
		params.SetActive(params.SepoliaConfig().Copy())
	} else if network == "prater" {
		params.SetActive(params.PraterConfig().Copy())
	} else {
		params.SetActive(params.MainnetConfig().Copy())
	}

	startSlot := epoch * 32
	endSlot := startSlot + 31

	if s == nil {
		stateId := fmt.Sprintf("%d", startSlot-1)
		if startSlot == 0 {
			stateId = "genesis"
			startSlot = 1
		}
		logrus.Infof("fetching state for slot %v from backend node", stateId)

		stateData, err := clClient.GetState(ctx, beacon.StateOrBlockId(stateId))
		if err != nil {
			return nil, err
		}

		vu, err := detect.FromState(stateData)
		if err != nil {
			return nil, err
		}

		s, err = vu.UnmarshalBeaconState(stateData)
		if err != nil {
			return nil, err
		}
		logrus.Infof("state is at slot %v", s.Slot())
	} else {
		s = s.Copy()
	}

	g := new(errgroup.Group)
	g.SetLimit(10)

	blocks := make(map[uint64][]byte)
	blocksMux := &sync.Mutex{}

	for i := startSlot; i <= endSlot; i++ {
		i := i

		g.Go(func() error {
			data, err := clClient.GetBlock(ctx, beacon.StateOrBlockId(fmt.Sprintf("%d", i)))
			if err != nil {
				if strings.Contains(err.Error(), "NOT_FOUND") {
					return nil
				}
				return err
			}

			blocksMux.Lock()
			blocks[i] = data
			blocksMux.Unlock()

			return nil
		})
	}
	err := g.Wait()
	if err != nil {
		return nil, err
	}

	// retrieve duties for the provided epoch
	committeeAssignments, proposerIndexToSlots, err := helpers.CommitteeAssignments(ctx, s.Copy(), types.Epoch(epoch))
	if err != nil {
		return nil, err
	}
	data.CommitteeAssignments = committeeAssignments
	data.ProposerAssignments = proposerIndexToSlots

	tracer.SetEpochDuties(s, committeeAssignments, proposerIndexToSlots)

	for i := startSlot; i <= endSlot; i++ {
		// logrus.Infof("processing slot %v, state is at slot %v", i, s.Slot())

		s, err = stategen.ReplayProcessSlots(ctx, s, types.Slot(i))
		if err != nil {
			return nil, err
		}

		serializedBlock := blocks[i] // blocks[i] contains the ssz serialized raw signed block
		if serializedBlock != nil {
			vu, err := detect.FromForkVersion(bytesutil.ToBytes4(s.Fork().CurrentVersion))
			if err != nil {
				logrus.Fatal(err)
			}

			b, err := vu.UnmarshalBeaconBlock(serializedBlock)

			if err != nil {
				return nil, err
			}
			s, err = transition.ProcessBlockForStateRoot(ctx, s, b)
			if err != nil {
				return nil, err
			}

			b.Block().Body().SyncAggregate()
			tracer.SetProposer(s, b.Block().ProposerIndex(), serializedBlock != nil)

			for _, att := range b.Block().Body().Attestations() {
				committee, err := helpers.BeaconCommitteeFromState(ctx, s, att.Data.Slot, att.Data.CommitteeIndex)
				if err != nil {
					return nil, err
				}

				indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
				if err != nil {
					return nil, err
				}

				tracer.SetAttestation(s, indexedAtt)
			}

			currentSyncCommittee, err := s.CurrentSyncCommittee()
			if err != nil {
				if err.Error() == "CurrentSyncCommittee is not supported for phase0" {
					continue
				} else {
					return nil, err
				}
			}

			sa, err := b.Block().Body().SyncAggregate()

			if err != nil {
				return nil, err
			}

			for i := uint64(0); i < sa.SyncCommitteeBits.Len(); i++ {
				vIdx, exists := s.ValidatorIndexByPubkey(bytesutil.ToBytes48(currentSyncCommittee.Pubkeys[i]))

				if !exists {
					return nil, fmt.Errorf("validator public key does not exist in state")
				}

				tracer.SetSyncDuty(s, vIdx, sa.SyncCommitteeBits.BitAt(i))
			}
		}
	}

	tracer.SetBalances(s)

	targetIdx := params.BeaconConfig().TimelyTargetFlagIndex
	pp, err := s.PreviousEpochParticipation()
	if err != nil {
		if err.Error() != "PreviousEpochParticipation is not supported for phase0" {
			return nil, err
		}
	} else {
		previousEpoch := epoch - 1
		for i, validator := range s.Validators() {
			farFutureEpoch := params.BeaconConfig().FarFutureEpoch

			// Pending.
			if uint64(validator.ActivationEpoch) > epoch {
				if validator.ActivationEligibilityEpoch < farFutureEpoch {
					data.NumActivatingValidators++
				}
			}

			// Exiting / Slashed.
			if uint64(validator.ActivationEpoch) <= epoch && epoch < uint64(validator.ExitEpoch) {
				if validator.ExitEpoch < farFutureEpoch {
					data.NumExitingValidators++
				}
			}
			active := uint64(validator.ActivationEpoch) <= previousEpoch && previousEpoch < uint64(validator.ExitEpoch)
			if active && !validator.Slashed {
				data.PreviousEpochActiveGWei, err = math.Add64(data.PreviousEpochActiveGWei, validator.EffectiveBalance)
				if err != nil {
					return nil, err
				}
			}

			if ((pp[i] >> targetIdx) & 1) == 1 {
				data.PreviousEpochVotedGWei, err = math.Add64(data.PreviousEpochVotedGWei, validator.EffectiveBalance)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	data.State = s
	data.Validators = tracer.GetData()

	return data, nil
}
