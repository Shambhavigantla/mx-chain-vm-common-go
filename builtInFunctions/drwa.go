package builtInFunctions

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	vmcommon "github.com/multiversx/mx-chain-vm-common-go"
)

const (
	drwaTokenPolicyPrefix  = "drwa:token:"
	drwaHolderMirrorPrefix = "drwa:holder:"
	drwaReadGasUnits       = 1
)

var (
	errDRWAKYCRequired        = errors.New("DRWA_KYC_REQUIRED")
	errDRWAAMLBlocked         = errors.New("DRWA_AML_BLOCKED")
	errDRWAInvestorClass      = errors.New("DRWA_INVESTOR_CLASS_BLOCKED")
	errDRWAJurisdiction       = errors.New("DRWA_JURISDICTION_BLOCKED")
	errDRWATokenPaused        = errors.New("DRWA_TOKEN_PAUSED")
	errDRWATransferLocked     = errors.New("DRWA_TRANSFER_LOCKED")
	errDRWAReceiveLocked      = errors.New("DRWA_RECEIVE_LOCKED")
	errDRWAAuditorRequired    = errors.New("DRWA_AUDITOR_REQUIRED")
	errDRWAAssetExpired       = errors.New("DRWA_ASSET_EXPIRED")
	errDRWANilAccountsAdapter = errors.New("nil DRWA accounts adapter")
)

type drwaDecision struct {
	Allowed    bool
	DenialCode error
}

type drwaTokenPolicyView struct {
	DRWAEnabled               bool            `json:"drwa_enabled"`
	GlobalPause               bool            `json:"global_pause"`
	StrictAuditorMode         bool            `json:"strict_auditor_mode"`
	MetadataProtectionEnabled bool            `json:"metadata_protection_enabled"`
	AllowedInvestorClasses    map[string]bool `json:"allowed_investor_classes,omitempty"`
	AllowedJurisdictions      map[string]bool `json:"allowed_jurisdictions,omitempty"`
}

type drwaHolderMirrorView struct {
	KYCStatus         string `json:"kyc_status"`
	AMLStatus         string `json:"aml_status"`
	InvestorClass     string `json:"investor_class,omitempty"`
	JurisdictionCode  string `json:"jurisdiction_code,omitempty"`
	ExpiryRound       uint64 `json:"expiry_round,omitempty"`
	// TransferLocked and ReceiveLocked are top-level fields whose JSON tags
	// match the DrwaHolderMirror struct fields serialized by the Rust contracts.
	// They must NOT be stored in a nested map — json.Unmarshal cannot populate
	// map entries from top-level JSON keys.
	TransferLocked    bool `json:"transfer_locked,omitempty"`
	ReceiveLocked     bool `json:"receive_locked,omitempty"`
	AuditorAuthorized bool `json:"auditor_authorized,omitempty"`
}

type drwaStateReader interface {
	GetTokenPolicy(tokenIdentifier []byte) (*drwaTokenPolicyView, error)
	GetHolderMirror(tokenIdentifier []byte, address []byte, currentAccount vmcommon.UserAccountHandler) (*drwaHolderMirrorView, error)
}

type drwaAccountsReader struct {
	accounts vmcommon.AccountsAdapter
}

type drwaStoredValue struct {
	Version uint64 `json:"version"`
	Body    []byte `json:"body"`
}

func newDRWAAccountsReader(accounts vmcommon.AccountsAdapter) (*drwaAccountsReader, error) {
	if accounts == nil || accounts.IsInterfaceNil() {
		return nil, errDRWANilAccountsAdapter
	}

	return &drwaAccountsReader{accounts: accounts}, nil
}

func buildDRWATokenPolicyKey(tokenIdentifier []byte) []byte {
	return []byte(drwaTokenPolicyPrefix + string(tokenIdentifier) + ":policy")
}

func buildDRWAHolderMirrorKey(tokenIdentifier []byte, address []byte) []byte {
	return []byte(drwaHolderMirrorPrefix + string(tokenIdentifier) + ":" + string(address))
}

func (d *drwaAccountsReader) GetTokenPolicy(tokenIdentifier []byte) (*drwaTokenPolicyView, error) {
	systemAccount, err := getSystemAccount(d.accounts)
	if err != nil {
		return nil, err
	}

	data, _, err := systemAccount.AccountDataHandler().RetrieveValue(buildDRWATokenPolicyKey(tokenIdentifier))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	policy := &drwaTokenPolicyView{}
	err = decodeDRWAStoredJSON(data, policy)
	if err != nil {
		return nil, fmt.Errorf("drwa token policy unmarshal: %w", err)
	}

	return policy, nil
}

func (d *drwaAccountsReader) GetHolderMirror(tokenIdentifier []byte, address []byte, currentAccount vmcommon.UserAccountHandler) (*drwaHolderMirrorView, error) {
	account, err := d.loadUserAccount(address, currentAccount)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, nil
	}

	data, _, err := account.AccountDataHandler().RetrieveValue(buildDRWAHolderMirrorKey(tokenIdentifier, address))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	holder := &drwaHolderMirrorView{}
	err = decodeDRWAStoredJSON(data, holder)
	if err != nil {
		return nil, fmt.Errorf("drwa holder mirror unmarshal: %w", err)
	}

	return holder, nil
}

func (d *drwaAccountsReader) loadUserAccount(address []byte, currentAccount vmcommon.UserAccountHandler) (vmcommon.UserAccountHandler, error) {
	if currentAccount != nil && !currentAccount.IsInterfaceNil() && string(currentAccount.AddressBytes()) == string(address) {
		return currentAccount, nil
	}

	accountHandler, err := d.accounts.LoadAccount(address)
	if err != nil {
		return nil, err
	}

	userAccount, ok := accountHandler.(vmcommon.UserAccountHandler)
	if !ok {
		return nil, ErrWrongTypeAssertion
	}

	return userAccount, nil
}

func decodeDRWAStoredJSON(data []byte, destination interface{}) error {
	storedValue := &drwaStoredValue{}
	var err error
	if jsonErr := json.Unmarshal(data, storedValue); jsonErr == nil && len(storedValue.Body) > 0 {
		err = decodeDRWABody(storedValue.Body, destination)
	} else {
		err = decodeDRWABody(data, destination)
	}
	if err != nil {
		recordDRWAGateMetric(drwaGateMetricDecodeFailure)
	}
	return err
}

func decodeDRWABody(data []byte, destination interface{}) error {
	// Bodies that start with '{' are JSON-format.  Do not fall back to the
	// binary decoder when JSON parsing fails: a corrupt JSON body must surface
	// as a parse error.  Silently re-routing to the binary decoder would drop
	// AllowedInvestorClasses / AllowedJurisdictions enforcement entirely.
	if len(data) > 0 && data[0] == '{' {
		return json.Unmarshal(data, destination)
	}

	if err := json.Unmarshal(data, destination); err == nil {
		return nil
	}

	switch typedDestination := destination.(type) {
	case *drwaTokenPolicyView:
		return decodeDRWABinaryTokenPolicy(data, typedDestination)
	case *drwaHolderMirrorView:
		return decodeDRWABinaryHolderMirror(data, typedDestination)
	default:
		return json.Unmarshal(data, destination)
	}
}

func decodeDRWABinaryTokenPolicy(data []byte, destination *drwaTokenPolicyView) error {
	if len(data) < 12 {
		return errors.New("invalid DRWA binary token policy payload")
	}

	destination.DRWAEnabled = data[0] == 1
	destination.GlobalPause = data[1] == 1
	destination.StrictAuditorMode = data[2] == 1
	destination.MetadataProtectionEnabled = data[3] == 1

	return nil
}

func decodeDRWABinaryHolderMirror(data []byte, destination *drwaHolderMirrorView) error {
	cursor := 0
	if len(data) < 8 {
		return errors.New("invalid DRWA binary holder payload")
	}

	cursor += 8

	kycStatus, nextCursor, err := readDRWABinaryField(data, cursor)
	if err != nil {
		return err
	}
	cursor = nextCursor

	amlStatus, nextCursor, err := readDRWABinaryField(data, cursor)
	if err != nil {
		return err
	}
	cursor = nextCursor

	investorClass, nextCursor, err := readDRWABinaryField(data, cursor)
	if err != nil {
		return err
	}
	cursor = nextCursor

	jurisdictionCode, nextCursor, err := readDRWABinaryField(data, cursor)
	if err != nil {
		return err
	}
	cursor = nextCursor

	if len(data[cursor:]) < 11 {
		return errors.New("invalid DRWA binary holder trailer")
	}

	destination.KYCStatus = string(kycStatus)
	destination.AMLStatus = string(amlStatus)
	destination.InvestorClass = string(investorClass)
	destination.JurisdictionCode = string(jurisdictionCode)
	destination.ExpiryRound = binary.BigEndian.Uint64(data[cursor : cursor+8])
	destination.TransferLocked = data[cursor+8] == 1
	destination.ReceiveLocked = data[cursor+9] == 1
	destination.AuditorAuthorized = data[cursor+10] == 1

	return nil
}

func readDRWABinaryField(data []byte, cursor int) ([]byte, int, error) {
	if len(data[cursor:]) < 4 {
		return nil, cursor, errors.New("invalid DRWA binary field length")
	}

	fieldLength := int(binary.BigEndian.Uint32(data[cursor : cursor+4]))
	cursor += 4
	if len(data[cursor:]) < fieldLength {
		return nil, cursor, errors.New("invalid DRWA binary field body")
	}

	field := append([]byte(nil), data[cursor:cursor+fieldLength]...)
	cursor += fieldLength

	return field, cursor, nil
}

func isDRWAEnforcementEnabled(enableEpochsHandler vmcommon.EnableEpochsHandler) bool {
	if enableEpochsHandler == nil || enableEpochsHandler.IsInterfaceNil() {
		return false
	}

	return enableEpochsHandler.IsFlagEnabled(DRWAEnforcementFlag)
}

func computeDRWAReadGasCost(baseCost vmcommon.BaseOperationCost, fallbackCost uint64, reads uint64) uint64 {
	if reads == 0 {
		return 0
	}

	unitCost := baseCost.DataCopyPerByte
	if unitCost == 0 {
		unitCost = fallbackCost
	}
	if unitCost == 0 {
		return 0
	}

	return reads * unitCost * drwaReadGasUnits
}

func isDRWARegulatedToken(reader drwaStateReader, tokenIdentifier []byte) (bool, *drwaTokenPolicyView, error) {
	if reader == nil {
		return false, nil, nil
	}

	policy, err := reader.GetTokenPolicy(tokenIdentifier)
	if err != nil {
		return false, nil, err
	}
	if policy == nil || !policy.DRWAEnabled {
		return false, nil, nil
	}

	return true, policy, nil
}

func validateDRWASender(policy *drwaTokenPolicyView, holder *drwaHolderMirrorView, now uint64) drwaDecision {
	if policy == nil || !policy.DRWAEnabled {
		return drwaDecision{Allowed: true}
	}
	if policy.GlobalPause {
		return drwaDecision{DenialCode: errDRWATokenPaused}
	}
	if holder == nil || holder.KYCStatus != "approved" {
		return drwaDecision{DenialCode: errDRWAKYCRequired}
	}
	if holder.AMLStatus == "blocked" {
		return drwaDecision{DenialCode: errDRWAAMLBlocked}
	}
	if holder.ExpiryRound > 0 && now > holder.ExpiryRound {
		return drwaDecision{DenialCode: errDRWAAssetExpired}
	}
	if holder.TransferLocked {
		return drwaDecision{DenialCode: errDRWATransferLocked}
	}
	if len(policy.AllowedInvestorClasses) > 0 && !policy.AllowedInvestorClasses[holder.InvestorClass] {
		return drwaDecision{DenialCode: errDRWAInvestorClass}
	}
	if len(policy.AllowedJurisdictions) > 0 && !policy.AllowedJurisdictions[holder.JurisdictionCode] {
		return drwaDecision{DenialCode: errDRWAJurisdiction}
	}

	return drwaDecision{Allowed: true}
}

func validateDRWAReceiver(policy *drwaTokenPolicyView, holder *drwaHolderMirrorView, now uint64) drwaDecision {
	if policy == nil || !policy.DRWAEnabled {
		return drwaDecision{Allowed: true}
	}
	if policy.GlobalPause {
		return drwaDecision{DenialCode: errDRWATokenPaused}
	}
	if holder == nil || holder.KYCStatus != "approved" {
		return drwaDecision{DenialCode: errDRWAKYCRequired}
	}
	if holder.AMLStatus == "blocked" {
		return drwaDecision{DenialCode: errDRWAAMLBlocked}
	}
	if holder.ExpiryRound > 0 && now > holder.ExpiryRound {
		return drwaDecision{DenialCode: errDRWAAssetExpired}
	}
	if holder.ReceiveLocked {
		return drwaDecision{DenialCode: errDRWAReceiveLocked}
	}
	if len(policy.AllowedInvestorClasses) > 0 && !policy.AllowedInvestorClasses[holder.InvestorClass] {
		return drwaDecision{DenialCode: errDRWAInvestorClass}
	}
	if len(policy.AllowedJurisdictions) > 0 && !policy.AllowedJurisdictions[holder.JurisdictionCode] {
		return drwaDecision{DenialCode: errDRWAJurisdiction}
	}

	return drwaDecision{Allowed: true}
}

func validateDRWAMetadataUpdate(policy *drwaTokenPolicyView, auditorAuthorized bool) drwaDecision {
	if policy == nil || !policy.DRWAEnabled {
		return drwaDecision{Allowed: true}
	}
	if !policy.MetadataProtectionEnabled {
		return drwaDecision{Allowed: true}
	}
	if policy.StrictAuditorMode && !auditorAuthorized {
		return drwaDecision{DenialCode: errDRWAAuditorRequired}
	}

	return drwaDecision{Allowed: true}
}

func evaluateDRWASenderTransfer(reader drwaStateReader, tokenID []byte, senderAddr []byte, senderAccount vmcommon.UserAccountHandler, now uint64) (bool, error) {
	regulated, policy, err := isDRWARegulatedToken(reader, tokenID)
	if err != nil || !regulated {
		return regulated, err
	}

	holder, err := reader.GetHolderMirror(tokenID, senderAddr, senderAccount)
	if err != nil {
		return true, err
	}

	decision := validateDRWASender(policy, holder, now)
	if !decision.Allowed {
		if m := drwaDenialMetric(decision.DenialCode); m != "" {
			recordDRWAGateMetric(m)
		}
		return true, decision.DenialCode
	}

	return true, nil
}

func checkDRWASenderTransfer(reader drwaStateReader, tokenID []byte, senderAddr []byte, senderAccount vmcommon.UserAccountHandler, now uint64) error {
	_, err := evaluateDRWASenderTransfer(reader, tokenID, senderAddr, senderAccount, now)
	return err
}

func evaluateDRWAReceiverTransfer(reader drwaStateReader, tokenID []byte, receiverAddr []byte, receiverAccount vmcommon.UserAccountHandler, now uint64) (bool, error) {
	regulated, policy, err := isDRWARegulatedToken(reader, tokenID)
	if err != nil || !regulated {
		return regulated, err
	}

	holder, err := reader.GetHolderMirror(tokenID, receiverAddr, receiverAccount)
	if err != nil {
		return true, err
	}

	decision := validateDRWAReceiver(policy, holder, now)
	if !decision.Allowed {
		if m := drwaDenialMetric(decision.DenialCode); m != "" {
			recordDRWAGateMetric(m)
		}
		return true, decision.DenialCode
	}

	return true, nil
}

func checkDRWAReceiverTransfer(reader drwaStateReader, tokenID []byte, receiverAddr []byte, receiverAccount vmcommon.UserAccountHandler, now uint64) error {
	_, err := evaluateDRWAReceiverTransfer(reader, tokenID, receiverAddr, receiverAccount, now)
	return err
}

func evaluateDRWAMetadataUpdate(reader drwaStateReader, tokenID []byte, callerAddr []byte, callerAccount vmcommon.UserAccountHandler) (bool, error) {
	regulated, policy, err := isDRWARegulatedToken(reader, tokenID)
	if err != nil || !regulated {
		return regulated, err
	}

	holder, err := reader.GetHolderMirror(tokenID, callerAddr, callerAccount)
	if err != nil {
		return true, err
	}

	auditorAuthorized := holder != nil && holder.AuditorAuthorized
	decision := validateDRWAMetadataUpdate(policy, auditorAuthorized)
	if !decision.Allowed {
		if m := drwaDenialMetric(decision.DenialCode); m != "" {
			recordDRWAGateMetric(m)
		}
		return true, decision.DenialCode
	}

	return true, nil
}

func checkDRWAMetadataUpdate(reader drwaStateReader, tokenID []byte, callerAddr []byte, callerAccount vmcommon.UserAccountHandler) error {
	_, err := evaluateDRWAMetadataUpdate(reader, tokenID, callerAddr, callerAccount)
	return err
}
