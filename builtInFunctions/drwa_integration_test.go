package builtInFunctions

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/multiversx/mx-chain-core-go/core"
	"github.com/multiversx/mx-chain-core-go/data/esdt"
	vm "github.com/multiversx/mx-chain-core-go/data/vm"
	vmcommon "github.com/multiversx/mx-chain-vm-common-go"
	"github.com/multiversx/mx-chain-vm-common-go/mock"
	"github.com/stretchr/testify/require"
)

func drwaEnabledEpochsHandler(extraFlags ...core.EnableEpochFlag) *mock.EnableEpochsHandlerStub {
	return &mock.EnableEpochsHandlerStub{
		IsFlagEnabledCalled: func(flag core.EnableEpochFlag) bool {
			if flag == DRWAEnforcementFlag {
				return true
			}
			for _, extraFlag := range extraFlags {
				if flag == extraFlag {
					return true
				}
			}

			return false
		},
	}
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWAAllowsApprovedSameShardTransfer(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolder(t, sender, "CARBON-123", "sender", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})
	mustSaveDRWAHolder(t, receiver, "CARBON-123", "receiver", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	output, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, vmcommon.Ok, output.ReturnCode)
	require.Equal(t, uint64(50), output.GasRemaining)
}

func TestESDTTransfer_ProcessBuiltinFunction_AllowsWhenTokenPolicyMissing(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "PLAIN-123", 10)

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("PLAIN-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	output, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, vmcommon.Ok, output.ReturnCode)
}

func TestESDTTransfer_ProcessBuiltinFunction_SkipsDRWAWhenFlagDisabled(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		&mock.EnableEpochsHandlerStub{},
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolder(t, sender, "CARBON-123", "sender", &drwaHolderMirrorView{
		KYCStatus: "pending",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	output, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, vmcommon.Ok, output.ReturnCode)
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWADeniesSender(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolder(t, sender, "CARBON-123", "sender", &drwaHolderMirrorView{
		KYCStatus: "pending",
		AMLStatus: "approved",
	})
	mustSaveDRWAHolder(t, receiver, "CARBON-123", "receiver", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAKYCRequired)
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWADeniesSenderFromBinaryStoredMirror(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicyBinary(t, systemAcc, "CARBON-123", true, false, false, false, 1)

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolderBinary(t, sender, "CARBON-123", "sender", 1, "pending", "approved", "accredited", "SG", 0, false, false, true)
	mustSaveDRWAHolderBinary(t, receiver, "CARBON-123", "receiver", 1, "approved", "approved", "accredited", "SG", 0, false, false, true)

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAKYCRequired)
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWADeniesWhenHolderMirrorMissing(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolder(t, receiver, "CARBON-123", "receiver", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAKYCRequired)
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWADeniesSenderWhenPolicyPaused(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
		GlobalPause: true,
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolder(t, sender, "CARBON-123", "sender", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})
	mustSaveDRWAHolder(t, receiver, "CARBON-123", "receiver", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWATokenPaused)
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWADeniesSenderWhenExpired(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	require.NoError(t, transferFunc.SetBlockchainHook(&mock.BlockDataHandlerStub{
		CurrentRoundCalled: func() uint64 {
			return 100
		},
	}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, "CARBON-123", 10)
	mustSaveDRWAHolder(t, sender, "CARBON-123", "sender", &drwaHolderMirrorView{
		KYCStatus:   "approved",
		AMLStatus:   "approved",
		ExpiryRound: 99,
	})
	mustSaveDRWAHolder(t, receiver, "CARBON-123", "receiver", &drwaHolderMirrorView{
		KYCStatus: "approved",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAAssetExpired)
}

func TestESDTNFTTransfer_ProcessBuiltinFunction_DRWADeniesReceiverOnDestination(t *testing.T) {
	t.Parallel()

	globalSettingsHandler := &mock.GlobalSettingsHandlerStub{}
	enableEpochsHandler := drwaEnabledEpochsHandler(SaveToSystemAccountFlag, CheckCorrectTokenIDForTransferRoleFlag)

	nftTransfer, _ := createNFTTransferAndStorageHandler(0, 2, globalSettingsHandler, enableEpochsHandler)
	nftTransfer.SetDRWAReader(mustCreateDRWAReader(t, nftTransfer.accounts))
	require.NoError(t, nftTransfer.SetPayableChecker(&mock.PayableHandlerStub{
		IsPayableCalled: func(address []byte) (bool, error) {
			return true, nil
		},
	}))

	systemAcc, err := nftTransfer.accounts.LoadAccount(vmcommon.SystemAccountAddress)
	require.NoError(t, err)
	mustSaveDRWATokenPolicy(t, systemAcc.(vmcommon.UserAccountHandler), "HOTEL-1", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	destinationAcc, err := nftTransfer.accounts.LoadAccount([]byte("destination1"))
	require.NoError(t, err)
	mustSaveDRWAHolder(t, destinationAcc.(vmcommon.UserAccountHandler), "HOTEL-1", "destination1", &drwaHolderMirrorView{
		KYCStatus: "pending",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender0"),
			Arguments: [][]byte{
				[]byte("HOTEL-1"),
				big.NewInt(1).Bytes(),
				big.NewInt(1).Bytes(),
				zeroByteArray,
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
		},
		RecipientAddr: []byte("destination1"),
	}

	_, err = nftTransfer.ProcessBuiltinFunction(nil, destinationAcc.(vmcommon.UserAccountHandler), vmInput)
	require.ErrorIs(t, err, errDRWAKYCRequired)
}

func TestESDTNFTTransfer_ProcessBuiltinFunction_AllowsWhenTokenPolicyMissing(t *testing.T) {
	t.Parallel()

	globalSettingsHandler := &mock.GlobalSettingsHandlerStub{}
	enableEpochsHandler := drwaEnabledEpochsHandler(SaveToSystemAccountFlag, CheckCorrectTokenIDForTransferRoleFlag)

	nftTransfer, _ := createNFTTransferAndStorageHandler(0, 2, globalSettingsHandler, enableEpochsHandler)
	nftTransfer.SetDRWAReader(mustCreateDRWAReader(t, nftTransfer.accounts))

	destinationAcc, err := nftTransfer.accounts.LoadAccount([]byte("destination1"))
	require.NoError(t, err)

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender0"),
			Arguments: [][]byte{
				[]byte("PLAIN-NFT"),
				big.NewInt(1).Bytes(),
				big.NewInt(1).Bytes(),
				zeroByteArray,
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
		},
		RecipientAddr: []byte("destination1"),
	}

	_, err = nftTransfer.ProcessBuiltinFunction(nil, destinationAcc.(vmcommon.UserAccountHandler), vmInput)
	require.ErrorIs(t, err, ErrAccountNotPayable)
}

func TestESDTNFTMultiTransfer_ProcessBuiltinFunction_AllowsWhenTokenPolicyMissing(t *testing.T) {
	t.Parallel()

	multiTransfer := createESDTNFTMultiTransferWithMockArguments(0, 1, &mock.GlobalSettingsHandlerStub{})
	multiTransfer.enableEpochsHandler = drwaEnabledEpochsHandler(ESDTNFTImprovementV1Flag, CheckCorrectTokenIDForTransferRoleFlag)
	multiTransfer.SetDRWAReader(mustCreateDRWAReader(t, multiTransfer.accounts))

	payableChecker, err := NewPayableCheckFunc(
		&mock.PayableHandlerStub{
			IsPayableCalled: func(address []byte) (bool, error) {
				return true, nil
			},
		},
		&mock.EnableEpochsHandlerStub{
			IsFlagEnabledCalled: func(flag core.EnableEpochFlag) bool {
				return flag == FixAsyncCallbackCheckFlag || flag == CheckFunctionArgumentFlag
			},
		},
	)
	require.NoError(t, err)
	require.NoError(t, multiTransfer.SetPayableChecker(payableChecker))

	senderAddress := bytes.Repeat([]byte{2}, 32)
	destinationAddress := bytes.Repeat([]byte{0}, 32)
	destinationAddress[25] = 1

	senderAccount, err := multiTransfer.accounts.LoadAccount(senderAddress)
	require.NoError(t, err)
	destinationAccount, err := multiTransfer.accounts.LoadAccount(destinationAddress)
	require.NoError(t, err)

	createESDTNFTToken([]byte("PLAIN-MULTI"), core.Fungible, 0, big.NewInt(3), multiTransfer.marshaller, senderAccount.(vmcommon.UserAccountHandler))
	require.NoError(t, multiTransfer.accounts.SaveAccount(senderAccount))
	require.NoError(t, multiTransfer.accounts.SaveAccount(destinationAccount))
	_, _ = multiTransfer.accounts.Commit()

	senderAccount, err = multiTransfer.accounts.LoadAccount(senderAddress)
	require.NoError(t, err)
	destinationAccount, err = multiTransfer.accounts.LoadAccount(destinationAddress)
	require.NoError(t, err)

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  senderAddress,
			CallValue:   big.NewInt(0),
			GasProvided: 100000,
			Arguments: [][]byte{
				destinationAddress,
				big.NewInt(1).Bytes(),
				[]byte("PLAIN-MULTI"),
				big.NewInt(0).Bytes(),
				big.NewInt(1).Bytes(),
			},
		},
		RecipientAddr: senderAddress,
	}

	output, err := multiTransfer.ProcessBuiltinFunction(senderAccount.(vmcommon.UserAccountHandler), destinationAccount.(vmcommon.UserAccountHandler), vmInput)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, vmcommon.Ok, output.ReturnCode)

	testNFTTokenShouldExist(t, multiTransfer.marshaller, senderAccount, []byte("PLAIN-MULTI"), 0, big.NewInt(2))
	testNFTTokenShouldExist(t, multiTransfer.marshaller, destinationAccount, []byte("PLAIN-MULTI"), 0, big.NewInt(1))
}

func TestESDTTransfer_ProcessBuiltinFunction_DRWADeniesReceiverOnDestinationCrossShardPhase(t *testing.T) {
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, "CARBON-123", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	receiver := state["receiver"]
	mustSaveDRWAHolder(t, receiver, "CARBON-123", "receiver", &drwaHolderMirrorView{
		KYCStatus: "pending",
		AMLStatus: "approved",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr: []byte("sender"),
			Arguments: [][]byte{
				[]byte("CARBON-123"),
				big.NewInt(1).Bytes(),
			},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(nil, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAKYCRequired)
}

func TestESDTNFTTransfer_ProcessBuiltinFunction_DRWADeniesSenderOnCrossShardSenderPhase(t *testing.T) {
	t.Parallel()

	vmInput, sender, nftTransfer, _, _, _ := createSetupToSendNFTCrossShard(t)
	nftTransfer.enableEpochsHandler = drwaEnabledEpochsHandler(SendAlwaysFlag, SaveToSystemAccountFlag, CheckFrozenCollectionFlag)
	nftTransfer.SetDRWAReader(mustCreateDRWAReader(t, nftTransfer.accounts))

	systemAcc, err := nftTransfer.accounts.LoadAccount(vmcommon.SystemAccountAddress)
	require.NoError(t, err)
	mustSaveDRWATokenPolicy(t, systemAcc.(vmcommon.UserAccountHandler), "token", &drwaTokenPolicyView{
		DRWAEnabled: true,
	})

	mustSaveDRWAHolder(t, sender.(vmcommon.UserAccountHandler), "token", string(vmInput.CallerAddr), &drwaHolderMirrorView{
		KYCStatus: "pending",
		AMLStatus: "approved",
	})

	_, err = nftTransfer.ProcessBuiltinFunction(sender.(vmcommon.UserAccountHandler), nil, vmInput)
	require.ErrorIs(t, err, errDRWAKYCRequired)
}

func TestUpdateNFTAttributes_ProcessBuiltinFunction_DRWADeniesWithoutAuditorAuthorization(t *testing.T) {
	t.Parallel()

	globalSettingsHandler := &mock.GlobalSettingsHandlerStub{}
	enableEpochsHandler := drwaEnabledEpochsHandler(ESDTNFTImprovementV1Flag, SaveToSystemAccountFlag)
	rolesHandler := &mock.ESDTRoleHandlerStub{}

	userAcc := mock.NewAccountWrapMock([]byte("audited"))
	systemAcc := mock.NewUserAccount(vmcommon.SystemAccountAddress)
	accounts := &mock.AccountsStub{
		LoadAccountCalled: func(address []byte) (vmcommon.AccountHandler, error) {
			if string(address) == string(vmcommon.SystemAccountAddress) {
				return systemAcc, nil
			}
			if string(address) == "audited" {
				return userAcc, nil
			}
			return mock.NewUserAccount(address), nil
		},
	}

	esdtDataStorage := createNewESDTDataStorageHandlerWithArgs(globalSettingsHandler, accounts, enableEpochsHandler)
	updateFunc, _ := NewESDTNFTUpdateAttributesFunc(
		10,
		vmcommon.BaseOperationCost{},
		esdtDataStorage,
		globalSettingsHandler,
		rolesHandler,
		enableEpochsHandler,
		&mock.MarshalizerMock{},
	)
	updateFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	esdtData := &esdt.ESDigitalToken{
		TokenMetaData: &esdt.MetaData{Name: []byte("test")},
		Value:         big.NewInt(10),
	}
	esdtDataBytes, err := (&mock.MarshalizerMock{}).Marshal(esdtData)
	require.NoError(t, err)
	require.NoError(t, userAcc.AccountDataHandler().SaveKeyValue([]byte(core.ProtectedKeyPrefix+core.ESDTKeyIdentifier+"MRV-NFT"+string([]byte{1})), esdtDataBytes))

	mustSaveDRWATokenPolicy(t, systemAcc, "MRV-NFT", &drwaTokenPolicyView{
		DRWAEnabled:               true,
		MetadataProtectionEnabled: true,
		StrictAuditorMode:         true,
	})
	mustSaveDRWAHolder(t, userAcc, "MRV-NFT", "audited", &drwaHolderMirrorView{
		KYCStatus:         "approved",
		AMLStatus:         "approved",
		AuditorAuthorized: false,
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  []byte("audited"),
			CallValue:   big.NewInt(0),
			GasProvided: 1000,
			Arguments:   [][]byte{[]byte("MRV-NFT"), {1}, []byte("new-attrs")},
		},
		RecipientAddr: []byte("audited"),
	}

	_, err = updateFunc.ProcessBuiltinFunction(userAcc, nil, vmInput)
	require.ErrorIs(t, err, errDRWAAuditorRequired)
}

func TestUpdateNFTAttributes_ProcessBuiltinFunction_AllowsWhenTokenPolicyMissing(t *testing.T) {
	t.Parallel()

	globalSettingsHandler := &mock.GlobalSettingsHandlerStub{}
	enableEpochsHandler := drwaEnabledEpochsHandler(ESDTNFTImprovementV1Flag, SaveToSystemAccountFlag)
	rolesHandler := &mock.ESDTRoleHandlerStub{}

	userAcc := mock.NewAccountWrapMock([]byte("plain-holder"))
	systemAcc := mock.NewUserAccount(vmcommon.SystemAccountAddress)
	accounts := &mock.AccountsStub{
		LoadAccountCalled: func(address []byte) (vmcommon.AccountHandler, error) {
			if string(address) == string(vmcommon.SystemAccountAddress) {
				return systemAcc, nil
			}
			if string(address) == "plain-holder" {
				return userAcc, nil
			}
			return mock.NewUserAccount(address), nil
		},
	}

	esdtDataStorage := createNewESDTDataStorageHandlerWithArgs(globalSettingsHandler, accounts, enableEpochsHandler)
	updateFunc, _ := NewESDTNFTUpdateAttributesFunc(
		10,
		vmcommon.BaseOperationCost{},
		esdtDataStorage,
		globalSettingsHandler,
		rolesHandler,
		enableEpochsHandler,
		&mock.MarshalizerMock{},
	)
	updateFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	esdtData := &esdt.ESDigitalToken{
		TokenMetaData: &esdt.MetaData{Name: []byte("plain")},
		Value:         big.NewInt(10),
	}
	esdtDataBytes, err := (&mock.MarshalizerMock{}).Marshal(esdtData)
	require.NoError(t, err)
	require.NoError(t, userAcc.AccountDataHandler().SaveKeyValue([]byte(core.ProtectedKeyPrefix+core.ESDTKeyIdentifier+"PLAIN-NFT"+string([]byte{1})), esdtDataBytes))

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  []byte("plain-holder"),
			CallValue:   big.NewInt(0),
			GasProvided: 1000,
			Arguments:   [][]byte{[]byte("PLAIN-NFT"), {1}, []byte("new-attrs")},
		},
		RecipientAddr: []byte("plain-holder"),
	}

	output, err := updateFunc.ProcessBuiltinFunction(userAcc, nil, vmInput)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, vmcommon.Ok, output.ReturnCode)
}

func createDRWATestAccounts() (*mock.AccountsStub, map[string]vmcommon.UserAccountHandler) {
	state := map[string]vmcommon.UserAccountHandler{
		string(vmcommon.SystemAccountAddress): mock.NewUserAccount(vmcommon.SystemAccountAddress),
		"sender":                              mock.NewUserAccount([]byte("sender")),
		"receiver":                            mock.NewUserAccount([]byte("receiver")),
	}

	accounts := &mock.AccountsStub{
		LoadAccountCalled: func(address []byte) (vmcommon.AccountHandler, error) {
			account, ok := state[string(address)]
			if !ok {
				account = mock.NewUserAccount(address)
				state[string(address)] = account
			}
			return account, nil
		},
		GetExistingAccountCalled: func(address []byte) (vmcommon.AccountHandler, error) {
			account, ok := state[string(address)]
			if !ok {
				account = mock.NewUserAccount(address)
				state[string(address)] = account
			}
			return account, nil
		},
	}

	return accounts, state
}

func mustCreateDRWAReader(t *testing.T, accounts vmcommon.AccountsAdapter) drwaStateReader {
	t.Helper()

	reader, err := newDRWAAccountsReader(accounts)
	require.NoError(t, err)

	return reader
}

func mustSaveDRWATokenPolicy(t *testing.T, account vmcommon.UserAccountHandler, tokenID string, policy *drwaTokenPolicyView) {
	t.Helper()

	body, err := json.Marshal(policy)
	require.NoError(t, err)
	data, err := json.Marshal(&drwaStoredValue{
		Version: 1,
		Body:    body,
	})
	require.NoError(t, err)
	require.NoError(t, account.AccountDataHandler().SaveKeyValue(buildDRWATokenPolicyKey([]byte(tokenID)), data))
}

func mustSaveDRWATokenPolicyBinary(
	t *testing.T,
	account vmcommon.UserAccountHandler,
	tokenID string,
	drwaEnabled bool,
	globalPause bool,
	strictAuditorMode bool,
	metadataProtectionEnabled bool,
	version uint64,
) {
	t.Helper()

	body := make([]byte, 12)
	if drwaEnabled {
		body[0] = 1
	}
	if globalPause {
		body[1] = 1
	}
	if strictAuditorMode {
		body[2] = 1
	}
	if metadataProtectionEnabled {
		body[3] = 1
	}
	binary.BigEndian.PutUint64(body[4:], version)

	data, err := json.Marshal(&drwaStoredValue{
		Version: version,
		Body:    body,
	})
	require.NoError(t, err)
	require.NoError(t, account.AccountDataHandler().SaveKeyValue(buildDRWATokenPolicyKey([]byte(tokenID)), data))
}

func mustSaveDRWAHolder(t *testing.T, account vmcommon.UserAccountHandler, tokenID string, address string, holder *drwaHolderMirrorView) {
	t.Helper()

	body, err := json.Marshal(holder)
	require.NoError(t, err)
	data, err := json.Marshal(&drwaStoredValue{
		Version: 1,
		Body:    body,
	})
	require.NoError(t, err)
	require.NoError(t, account.AccountDataHandler().SaveKeyValue(buildDRWAHolderMirrorKey([]byte(tokenID), []byte(address)), data))
}

func mustSaveDRWAHolderBinary(
	t *testing.T,
	account vmcommon.UserAccountHandler,
	tokenID string,
	address string,
	version uint64,
	kycStatus string,
	amlStatus string,
	investorClass string,
	jurisdictionCode string,
	expiryRound uint64,
	transferLocked bool,
	receiveLocked bool,
	auditorAuthorized bool,
) {
	t.Helper()

	body := make([]byte, 0, 64)
	versionBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(versionBytes, version)
	body = append(body, versionBytes...)
	body = appendBinaryField(body, []byte(kycStatus))
	body = appendBinaryField(body, []byte(amlStatus))
	body = appendBinaryField(body, []byte(investorClass))
	body = appendBinaryField(body, []byte(jurisdictionCode))
	expiryBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(expiryBytes, expiryRound)
	body = append(body, expiryBytes...)
	if transferLocked {
		body = append(body, 1)
	} else {
		body = append(body, 0)
	}
	if receiveLocked {
		body = append(body, 1)
	} else {
		body = append(body, 0)
	}
	if auditorAuthorized {
		body = append(body, 1)
	} else {
		body = append(body, 0)
	}

	data, err := json.Marshal(&drwaStoredValue{
		Version: version,
		Body:    body,
	})
	require.NoError(t, err)
	require.NoError(t, account.AccountDataHandler().SaveKeyValue(buildDRWAHolderMirrorKey([]byte(tokenID), []byte(address)), data))
}

func appendBinaryField(destination []byte, value []byte) []byte {
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(value)))
	destination = append(destination, lengthBytes...)
	destination = append(destination, value...)
	return destination
}

func mustSaveESDTBalance(t *testing.T, account vmcommon.UserAccountHandler, tokenID string, value int64) {
	t.Helper()

	token := &esdt.ESDigitalToken{
		Value: big.NewInt(value),
	}
	data, err := (&mock.MarshalizerMock{}).Marshal(token)
	require.NoError(t, err)
	require.NoError(t, account.AccountDataHandler().SaveKeyValue([]byte(baseESDTKeyPrefix+tokenID), data))
}

// N-04: Investor class and jurisdiction enforcement integration tests.
// These exercise the full enforcement path from token policy → holder mirror →
// validateDRWASender decision, proving that AllowedInvestorClasses and
// AllowedJurisdictions are evaluated (not silently dropped via binary fallback).

func TestESDTTransferDRWAInvestorClassBlocked(t *testing.T) {
	const tokenID = "BOND1INVCLASS"
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	// Token restricts to "QIB" investor class only.
	mustSaveDRWATokenPolicy(t, systemAcc, tokenID, &drwaTokenPolicyView{
		DRWAEnabled:            true,
		AllowedInvestorClasses: map[string]bool{"QIB": true},
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, tokenID, 10)
	// Sender has investor class "RETAIL" — not in the allowed set.
	mustSaveDRWAHolder(t, sender, tokenID, "sender", &drwaHolderMirrorView{
		KYCStatus:     "approved",
		AMLStatus:     "approved",
		InvestorClass: "RETAIL",
	})
	mustSaveDRWAHolder(t, receiver, tokenID, "receiver", &drwaHolderMirrorView{
		KYCStatus:     "approved",
		AMLStatus:     "approved",
		InvestorClass: "QIB",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  []byte("sender"),
			Arguments:   [][]byte{[]byte(tokenID), big.NewInt(1).Bytes()},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAInvestorClass)
}

func TestESDTTransferDRWAJurisdictionBlocked(t *testing.T) {
	const tokenID = "BOND2JURISD"
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	// Token permits only "US" and "DE" jurisdictions.
	mustSaveDRWATokenPolicy(t, systemAcc, tokenID, &drwaTokenPolicyView{
		DRWAEnabled:          true,
		AllowedJurisdictions: map[string]bool{"US": true, "DE": true},
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, tokenID, 10)
	// Sender jurisdiction "CN" is not in the allowed set.
	mustSaveDRWAHolder(t, sender, tokenID, "sender", &drwaHolderMirrorView{
		KYCStatus:        "approved",
		AMLStatus:        "approved",
		JurisdictionCode: "CN",
	})
	mustSaveDRWAHolder(t, receiver, tokenID, "receiver", &drwaHolderMirrorView{
		KYCStatus:        "approved",
		AMLStatus:        "approved",
		JurisdictionCode: "US",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  []byte("sender"),
			Arguments:   [][]byte{[]byte(tokenID), big.NewInt(1).Bytes()},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	_, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.ErrorIs(t, err, errDRWAJurisdiction)
}

func TestESDTTransferDRWAInvestorClassAndJurisdictionAllowed(t *testing.T) {
	const tokenID = "BOND3BOTH"
	t.Parallel()

	accounts, state := createDRWATestAccounts()
	transferFunc, _ := NewESDTTransferFunc(
		10,
		&mock.MarshalizerMock{},
		&mock.GlobalSettingsHandlerStub{},
		&mock.ShardCoordinatorStub{},
		&mock.ESDTRoleHandlerStub{},
		drwaEnabledEpochsHandler(),
	)
	require.NoError(t, transferFunc.SetPayableChecker(&mock.PayableHandlerStub{}))
	transferFunc.SetDRWAReader(mustCreateDRWAReader(t, accounts))

	systemAcc := state[string(vmcommon.SystemAccountAddress)]
	mustSaveDRWATokenPolicy(t, systemAcc, tokenID, &drwaTokenPolicyView{
		DRWAEnabled:            true,
		AllowedInvestorClasses: map[string]bool{"QIB": true},
		AllowedJurisdictions:   map[string]bool{"US": true},
	})

	sender := state["sender"]
	receiver := state["receiver"]
	mustSaveESDTBalance(t, sender, tokenID, 10)
	mustSaveDRWAHolder(t, sender, tokenID, "sender", &drwaHolderMirrorView{
		KYCStatus:        "approved",
		AMLStatus:        "approved",
		InvestorClass:    "QIB",
		JurisdictionCode: "US",
	})
	mustSaveDRWAHolder(t, receiver, tokenID, "receiver", &drwaHolderMirrorView{
		KYCStatus:        "approved",
		AMLStatus:        "approved",
		InvestorClass:    "QIB",
		JurisdictionCode: "US",
	})

	vmInput := &vmcommon.ContractCallInput{
		VMInput: vmcommon.VMInput{
			CallerAddr:  []byte("sender"),
			Arguments:   [][]byte{[]byte(tokenID), big.NewInt(1).Bytes()},
			CallValue:   big.NewInt(0),
			GasProvided: 100,
			CallType:    vm.DirectCall,
		},
		RecipientAddr: []byte("receiver"),
	}

	output, err := transferFunc.ProcessBuiltinFunction(sender, receiver, vmInput)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, vmcommon.Ok, output.ReturnCode)
}
