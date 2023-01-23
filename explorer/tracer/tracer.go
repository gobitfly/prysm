package tracer

import (
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/state"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"github.com/sirupsen/logrus"
)

type RewardType uint64

const (
	AttestationReward RewardType = iota
	AttestationPenalty
	FinalityDelayPenalty
	ProposerSlashingInclusionReward
	ProposerAttestationInclusionReward
	ProposerSyncInclusionReward
	SyncCommitteeReward
	SyncCommitteePenalty
	SlashingReward
	SlashingPenalty
)

func (rt RewardType) String() string {
	switch rt {
	case AttestationReward:
		return "AttestationReward"
	case AttestationPenalty:
		return "AttestationPenalty"
	case FinalityDelayPenalty:
		return "FinalityDelayPenalty"
	case ProposerSlashingInclusionReward:
		return "ProposerSlashingInclusionReward"
	case ProposerSyncInclusionReward:
		return "ProposerSyncInclusionReward"
	case SyncCommitteeReward:
		return "SyncCommitteeReward"
	case SyncCommitteePenalty:
		return "SyncCommitteePenalty"
	case SlashingReward:
		return "SlashingReward"
	case SlashingPenalty:
		return "SlashingPenalty"
	}

	return ""
}

type ValidatorEpochData struct {
	Proposals        map[uint64]int64      // slot, true = proposed, false = missed
	SyncDuties       map[uint64]int64      // slot, true = duty done, false = missed
	Attestations     map[uint64]int64      // slot the validator attested for, slot the attestation was included
	Balance          uint64                // balance of the validator at the start of the epoch
	EffectiveBalance uint64                // effective balance of the validator at the start of the epoch
	IncomeDetails    *ValidatorEpochIncome // income details of the validator during the epoch
}

var data = make(map[types.ValidatorIndex]*ValidatorEpochData)

func ClearData() {
	data = make(map[types.ValidatorIndex]*ValidatorEpochData)
}

func GetData() map[types.ValidatorIndex]*ValidatorEpochData {
	dRet := make(map[types.ValidatorIndex]*ValidatorEpochData, len(data))
	for validator, ed := range data {
		dRet[validator] = ed
	}
	return dRet
}

func SetReward(beaconState state.BeaconState, validator types.ValidatorIndex, reward uint64, rewardType RewardType) {
	if reward == 0 {
		return
	}

	addToMap(validator)

	// if epoch == 57 {
	// 	logrus.Fatal(beaconState.Slot(), rewardType)
	// }
	switch rewardType {
	case AttestationReward:
		data[validator].IncomeDetails.AttestationReward += reward
	case ProposerAttestationInclusionReward:
		data[validator].IncomeDetails.ProposerAttestationInclusionReward += reward
	case ProposerSyncInclusionReward:
		data[validator].IncomeDetails.ProposerSyncInclusionReward += reward
	case ProposerSlashingInclusionReward:
		data[validator].IncomeDetails.ProposerSlashingInclusionReward += reward
	case SlashingReward:
		data[validator].IncomeDetails.SlashingReward += reward
	case SyncCommitteeReward:
		data[validator].IncomeDetails.SyncCommitteeReward += reward
	default:
		logrus.Fatal("reward type %v not defined", rewardType)
	}
	// log.WithFields(log.Fields{
	// 	"epoch":     epoch,
	// 	"validator": validator,
	// 	"reward":    reward,
	// 	"type":      rewardType,
	// }).Infof("setting reward")
	// switch rewardType {
	// case PROPOSER:

	// }
}

func SetPenalty(beaconState state.BeaconState, validator types.ValidatorIndex, reward uint64, rewardType RewardType) {
	if reward == 0 {
		return
	}

	addToMap(validator)

	switch rewardType {
	case AttestationPenalty:
		data[validator].IncomeDetails.AttestationPenalty += reward
	case SlashingPenalty:
		data[validator].IncomeDetails.SlashingPenalty += reward
	case SyncCommitteePenalty:
		data[validator].IncomeDetails.SyncCommitteePenalty += reward
	default:
		logrus.Fatal("reward type %v not defined", rewardType)
	}

	// log.WithFields(log.Fields{
	// 	"epoch":     epoch,
	// 	"validator": validator,
	// 	"reward":    reward,
	// 	"type":      rewardType,
	// }).Infof("setting penalty")
	// switch rewardType {
	// case PROPOSER:

	// }
}

func SetProposer(beaconState state.BeaconState, proposer types.ValidatorIndex, proposed bool) {
	addToMap(proposer)

	data[proposer].Proposals[uint64(beaconState.Slot())] = 1
}

func SetAttestation(beaconState state.BeaconState, att *ethpb.IndexedAttestation) {
	for _, v := range att.AttestingIndices {
		addToMap(types.ValidatorIndex(v))
		if data[types.ValidatorIndex(v)].Attestations[uint64(beaconState.Slot())] <= 0 {
			data[types.ValidatorIndex(v)].Attestations[uint64(att.Data.Slot)] = int64(beaconState.Slot())
		}
	}
}

func SetSyncDuty(beaconState state.BeaconState, validator types.ValidatorIndex, participated bool) {
	addToMap(validator)

	if participated {
		data[validator].SyncDuties[uint64(beaconState.Slot())] = 1
	} else {
		data[validator].SyncDuties[uint64(beaconState.Slot())] = -1
	}
}

func SetBalances(beaconState state.BeaconState) {
	for validator, b := range beaconState.Balances() {
		addToMap(types.ValidatorIndex(validator))
		data[types.ValidatorIndex(validator)].Balance = b
	}

	for validator, b := range beaconState.Validators() {
		addToMap(types.ValidatorIndex(validator))
		data[types.ValidatorIndex(validator)].EffectiveBalance = b.EffectiveBalance
	}
}

func SetEpochDuties(beaconState state.BeaconState, committeeAssignments map[types.ValidatorIndex]*helpers.CommitteeAssignmentContainer, proposerIndexToSlots map[types.ValidatorIndex][]types.Slot) {
	// set the proposer duties
	for v, slots := range proposerIndexToSlots {
		addToMap(types.ValidatorIndex(v))

		for _, slot := range slots {
			data[types.ValidatorIndex(v)].Proposals[uint64(slot)] = -1
		}
	}

	// set the attester duties
	for v, assignments := range committeeAssignments {
		addToMap(types.ValidatorIndex(v))

		data[types.ValidatorIndex(v)].Attestations[uint64(assignments.AttesterSlot)] = -1
	}
}

func addToMap(validator types.ValidatorIndex) {
	if data == nil {
		data = make(map[types.ValidatorIndex]*ValidatorEpochData)
	}

	if data[validator] == nil {
		data[validator] = &ValidatorEpochData{
			IncomeDetails: &ValidatorEpochIncome{},
			Proposals:     make(map[uint64]int64),
			SyncDuties:    make(map[uint64]int64),
			Attestations:  make(map[uint64]int64),
		}
	}
}
