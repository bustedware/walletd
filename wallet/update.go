package wallet

import (
	"fmt"

	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
)

type (
	// AddressBalance pairs an address with its balance.
	AddressBalance struct {
		Address types.Address `json:"address"`
		Balance
	}

	// An UpdateTx atomically updates the state of a store.
	UpdateTx interface {
		SiacoinStateElements() ([]types.StateElement, error)
		UpdateSiacoinStateElements([]types.StateElement) error

		SiafundStateElements() ([]types.StateElement, error)
		UpdateSiafundStateElements([]types.StateElement) error

		AddSiacoinElements([]types.SiacoinElement) error
		RemoveSiacoinElements([]types.SiacoinOutputID) error

		AddSiafundElements([]types.SiafundElement) error
		RemoveSiafundElements([]types.SiafundOutputID) error

		MaturedSiacoinElements(types.ChainIndex) ([]types.SiacoinElement, error)

		AddressRelevant(types.Address) (bool, error)
		AddressBalance(types.Address) (Balance, error)
		UpdateBalances([]AddressBalance) error
	}

	// An ApplyTx atomically applies a set of updates to a store.
	ApplyTx interface {
		UpdateTx

		AddEvents([]Event) error
	}

	// RevertTx atomically reverts an update from a store.
	RevertTx interface {
		UpdateTx

		RevertEvents(index types.ChainIndex) error
	}
)

// ApplyChainUpdates atomically applies a set of chain updates to a store
func ApplyChainUpdates(tx ApplyTx, updates []*chain.ApplyUpdate) error {
	var events []Event
	balances := make(map[types.Address]Balance)
	newSiacoinElements := make(map[types.SiacoinOutputID]types.SiacoinElement)
	newSiafundElements := make(map[types.SiafundOutputID]types.SiafundElement)
	spentSiacoinElements := make(map[types.SiacoinOutputID]bool)
	spentSiafundElements := make(map[types.SiafundOutputID]bool)

	updateBalance := func(addr types.Address, fn func(b *Balance)) error {
		balance, ok := balances[addr]
		if !ok {
			var err error
			balance, err = tx.AddressBalance(addr)
			if err != nil {
				return fmt.Errorf("failed to get address balance: %w", err)
			}
		}

		fn(&balance)
		balances[addr] = balance
		return nil
	}

	// fetch all siacoin and siafund state elements
	siacoinStateElements, err := tx.SiacoinStateElements()
	if err != nil {
		return fmt.Errorf("failed to get siacoin state elements: %w", err)
	}
	siafundStateElements, err := tx.SiafundStateElements()
	if err != nil {
		return fmt.Errorf("failed to get siafund state elements: %w", err)
	}

	for _, cau := range updates {
		// update the immature balance of each relevant address
		matured, err := tx.MaturedSiacoinElements(cau.State.Index)
		if err != nil {
			return fmt.Errorf("failed to get matured siacoin elements: %w", err)
		}
		for _, se := range matured {
			err := updateBalance(se.SiacoinOutput.Address, func(b *Balance) {
				b.ImmatureSiacoins = b.ImmatureSiacoins.Sub(se.SiacoinOutput.Value)
				b.Siacoins = b.Siacoins.Add(se.SiacoinOutput.Value)
			})
			if err != nil {
				return fmt.Errorf("failed to update address balance: %w", err)
			}
		}

		// determine which siacoin and siafund elements are ephemeral
		//
		// note: I thought we could use LeafIndex == EphemeralLeafIndex, but
		// it seems to be set before the subscriber is called.
		created := make(map[types.Hash256]bool)
		ephemeral := make(map[types.Hash256]bool)
		for _, txn := range cau.Block.Transactions {
			for i := range txn.SiacoinOutputs {
				created[types.Hash256(txn.SiacoinOutputID(i))] = true
			}
			for _, input := range txn.SiacoinInputs {
				ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
			}
			for i := range txn.SiafundOutputs {
				created[types.Hash256(txn.SiafundOutputID(i))] = true
			}
			for _, input := range txn.SiafundInputs {
				ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
			}
		}

		// add new siacoin elements to the store
		var siacoinElementErr error
		cau.ForEachSiacoinElement(func(se types.SiacoinElement, spent bool) {
			if siacoinElementErr != nil {
				return
			} else if ephemeral[se.ID] {
				return
			}

			relevant, err := tx.AddressRelevant(se.SiacoinOutput.Address)
			if err != nil {
				siacoinElementErr = fmt.Errorf("failed to check if address is relevant: %w", err)
				return
			} else if !relevant {
				return
			}

			if spent {
				delete(newSiacoinElements, types.SiacoinOutputID(se.ID))
				spentSiacoinElements[types.SiacoinOutputID(se.ID)] = true
			} else {
				newSiacoinElements[types.SiacoinOutputID(se.ID)] = se
			}

			err = updateBalance(se.SiacoinOutput.Address, func(b *Balance) {
				switch {
				case se.MaturityHeight > cau.State.Index.Height:
					b.ImmatureSiacoins = b.ImmatureSiacoins.Add(se.SiacoinOutput.Value)
				case spent:
					b.Siacoins = b.Siacoins.Sub(se.SiacoinOutput.Value)
				default:
					b.Siacoins = b.Siacoins.Add(se.SiacoinOutput.Value)
				}
			})
			if err != nil {
				siacoinElementErr = fmt.Errorf("failed to update address balance: %w", err)
				return
			}
		})
		if siacoinElementErr != nil {
			return fmt.Errorf("failed to add siacoin elements: %w", siacoinElementErr)
		}

		var siafundElementErr error
		cau.ForEachSiafundElement(func(se types.SiafundElement, spent bool) {
			if siafundElementErr != nil {
				return
			} else if ephemeral[se.ID] {
				return
			}

			relevant, err := tx.AddressRelevant(se.SiafundOutput.Address)
			if err != nil {
				siafundElementErr = fmt.Errorf("failed to check if address is relevant: %w", err)
				return
			} else if !relevant {
				return
			}

			if spent {
				delete(newSiafundElements, types.SiafundOutputID(se.ID))
				spentSiafundElements[types.SiafundOutputID(se.ID)] = true
			} else {
				newSiafundElements[types.SiafundOutputID(se.ID)] = se
			}

			err = updateBalance(se.SiafundOutput.Address, func(b *Balance) {
				if spent {
					if b.Siafunds < se.SiafundOutput.Value {
						panic(fmt.Errorf("negative siafund balance"))
					}
					b.Siafunds -= se.SiafundOutput.Value
				} else {
					b.Siafunds += se.SiafundOutput.Value
				}
			})
			if err != nil {
				siafundElementErr = fmt.Errorf("failed to update address balance: %w", err)
				return
			}
		})

		// add events
		relevant := func(addr types.Address) bool {
			relevant, err := tx.AddressRelevant(addr)
			if err != nil {
				panic(fmt.Errorf("failed to check if address is relevant: %w", err))
			}
			return relevant
		}
		if err != nil {
			return fmt.Errorf("failed to get applied events: %w", err)
		}
		events = append(events, AppliedEvents(cau.State, cau.Block, cau, relevant)...)

		// update siacoin element proofs
		for id := range newSiacoinElements {
			ele := newSiacoinElements[id]
			cau.UpdateElementProof(&ele.StateElement)
			newSiacoinElements[id] = ele
		}
		for i := range siacoinStateElements {
			cau.UpdateElementProof(&siacoinStateElements[i])
		}

		// update siafund element proofs
		for id := range newSiafundElements {
			ele := newSiafundElements[id]
			cau.UpdateElementProof(&ele.StateElement)
			newSiafundElements[id] = ele
		}
		for i := range siafundStateElements {
			cau.UpdateElementProof(&siafundStateElements[i])
		}
	}

	// update the address balances
	balanceChanges := make([]AddressBalance, 0, len(balances))
	for addr, balance := range balances {
		balanceChanges = append(balanceChanges, AddressBalance{
			Address: addr,
			Balance: balance,
		})
	}
	if err = tx.UpdateBalances(balanceChanges); err != nil {
		return fmt.Errorf("failed to update address balance: %w", err)
	}

	// add the new siacoin elements
	siacoinElements := make([]types.SiacoinElement, 0, len(newSiacoinElements))
	for _, ele := range newSiacoinElements {
		siacoinElements = append(siacoinElements, ele)
	}
	if err = tx.AddSiacoinElements(siacoinElements); err != nil {
		return fmt.Errorf("failed to add siacoin elements: %w", err)
	}

	// remove the spent siacoin elements
	siacoinOutputIDs := make([]types.SiacoinOutputID, 0, len(spentSiacoinElements))
	for id := range spentSiacoinElements {
		siacoinOutputIDs = append(siacoinOutputIDs, id)
	}
	if err = tx.RemoveSiacoinElements(siacoinOutputIDs); err != nil {
		return fmt.Errorf("failed to remove siacoin elements: %w", err)
	}

	// add the new siafund elements
	siafundElements := make([]types.SiafundElement, 0, len(newSiafundElements))
	for _, ele := range newSiafundElements {
		siafundElements = append(siafundElements, ele)
	}
	if err = tx.AddSiafundElements(siafundElements); err != nil {
		return fmt.Errorf("failed to add siafund elements: %w", err)
	}

	// remove the spent siafund elements
	siafundOutputIDs := make([]types.SiafundOutputID, 0, len(spentSiafundElements))
	for id := range spentSiafundElements {
		siafundOutputIDs = append(siafundOutputIDs, id)
	}
	if err = tx.RemoveSiafundElements(siafundOutputIDs); err != nil {
		return fmt.Errorf("failed to remove siafund elements: %w", err)
	}

	// add new events
	if err = tx.AddEvents(events); err != nil {
		return fmt.Errorf("failed to add events: %w", err)
	}

	// update the siacoin state elements
	filteredStateElements := siacoinStateElements[:0]
	for _, se := range siacoinStateElements {
		if _, ok := spentSiacoinElements[types.SiacoinOutputID(se.ID)]; !ok {
			filteredStateElements = append(filteredStateElements, se)
		}
	}
	err = tx.UpdateSiacoinStateElements(filteredStateElements)
	if err != nil {
		return fmt.Errorf("failed to update siacoin state elements: %w", err)
	}

	// update the siafund state elements
	filteredStateElements = siafundStateElements[:0]
	for _, se := range siafundStateElements {
		if _, ok := spentSiafundElements[types.SiafundOutputID(se.ID)]; !ok {
			filteredStateElements = append(filteredStateElements, se)
		}
	}
	if err = tx.UpdateSiafundStateElements(filteredStateElements); err != nil {
		return fmt.Errorf("failed to update siafund state elements: %w", err)
	}

	return nil
}

// RevertChainUpdate atomically reverts a chain update from a store
func RevertChainUpdate(tx RevertTx, cru *chain.RevertUpdate) error {
	balances := make(map[types.Address]Balance)

	var deletedSiacoinElements []types.SiacoinOutputID
	var addedSiacoinElements []types.SiacoinElement
	var deletedSiafundElements []types.SiafundOutputID
	var addedSiafundElements []types.SiafundElement

	updateBalance := func(addr types.Address, fn func(b *Balance)) error {
		balance, ok := balances[addr]
		if !ok {
			var err error
			balance, err = tx.AddressBalance(addr)
			if err != nil {
				return fmt.Errorf("failed to get address balance: %w", err)
			}
		}

		fn(&balance)
		balances[addr] = balance
		return nil
	}

	// determine which siacoin and siafund elements are ephemeral
	//
	// note: I thought we could use LeafIndex == EphemeralLeafIndex, but
	// it seems to be set before the subscriber is called.
	created := make(map[types.Hash256]bool)
	ephemeral := make(map[types.Hash256]bool)
	for _, txn := range cru.Block.Transactions {
		for i := range txn.SiacoinOutputs {
			created[types.Hash256(txn.SiacoinOutputID(i))] = true
		}
		for _, input := range txn.SiacoinInputs {
			ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
		}
		for i := range txn.SiafundOutputs {
			created[types.Hash256(txn.SiafundOutputID(i))] = true
		}
		for _, input := range txn.SiafundInputs {
			ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
		}
	}

	// revert the immature balance of each relevant address
	revertedIndex := types.ChainIndex{
		Height: cru.State.Index.Height + 1,
		ID:     cru.Block.ID(),
	}

	matured, err := tx.MaturedSiacoinElements(revertedIndex)
	if err != nil {
		return fmt.Errorf("failed to get matured siacoin elements: %w", err)
	}
	for _, se := range matured {
		err := updateBalance(se.SiacoinOutput.Address, func(b *Balance) {
			b.ImmatureSiacoins = b.ImmatureSiacoins.Add(se.SiacoinOutput.Value)
			b.Siacoins = b.Siacoins.Sub(se.SiacoinOutput.Value)
		})
		if err != nil {
			return fmt.Errorf("failed to update address balance: %w", err)
		}
	}

	var siacoinElementErr error
	cru.ForEachSiacoinElement(func(se types.SiacoinElement, spent bool) {
		if siacoinElementErr != nil {
			return
		}

		relevant, err := tx.AddressRelevant(se.SiacoinOutput.Address)
		if err != nil {
			siacoinElementErr = fmt.Errorf("failed to check if address is relevant: %w", err)
			return
		} else if !relevant {
			return
		} else if ephemeral[se.ID] {
			return
		}

		if spent {
			// re-add any spent siacoin elements
			addedSiacoinElements = append(addedSiacoinElements, se)
		} else {
			// delete any created siacoin elements
			deletedSiacoinElements = append(deletedSiacoinElements, types.SiacoinOutputID(se.ID))
		}

		siacoinElementErr = updateBalance(se.SiacoinOutput.Address, func(b *Balance) {
			switch {
			case se.MaturityHeight > cru.State.Index.Height:
				b.ImmatureSiacoins = b.ImmatureSiacoins.Sub(se.SiacoinOutput.Value)
			case spent:
				b.Siacoins = b.Siacoins.Add(se.SiacoinOutput.Value)
			default:
				b.Siacoins = b.Siacoins.Sub(se.SiacoinOutput.Value)
			}
		})
	})
	if siacoinElementErr != nil {
		return fmt.Errorf("failed to update address balance: %w", siacoinElementErr)
	}

	var siafundElementErr error
	cru.ForEachSiafundElement(func(se types.SiafundElement, spent bool) {
		if siafundElementErr != nil {
			return
		}

		relevant, err := tx.AddressRelevant(se.SiafundOutput.Address)
		if err != nil {
			siacoinElementErr = fmt.Errorf("failed to check if address is relevant: %w", err)
			return
		} else if !relevant {
			return
		} else if ephemeral[se.ID] {
			return
		}

		if spent {
			// re-add any spent siafund elements
			addedSiafundElements = append(addedSiafundElements, se)
		} else {
			// delete any created siafund elements
			deletedSiafundElements = append(deletedSiafundElements, types.SiafundOutputID(se.ID))
		}

		siafundElementErr = updateBalance(se.SiafundOutput.Address, func(b *Balance) {
			if spent {
				b.Siafunds += se.SiafundOutput.Value
			} else {
				b.Siafunds -= se.SiafundOutput.Value
			}
		})
	})
	if siafundElementErr != nil {
		return fmt.Errorf("failed to update address balance: %w", siafundElementErr)
	}

	balanceChanges := make([]AddressBalance, 0, len(balances))
	for addr, balance := range balances {
		balanceChanges = append(balanceChanges, AddressBalance{
			Address: addr,
			Balance: balance,
		})
	}
	if err := tx.UpdateBalances(balanceChanges); err != nil {
		return fmt.Errorf("failed to update address balance: %w", err)
	}

	// revert siacoin element changes
	if err := tx.AddSiacoinElements(addedSiacoinElements); err != nil {
		return fmt.Errorf("failed to add siacoin elements: %w", err)
	} else if err := tx.RemoveSiacoinElements(deletedSiacoinElements); err != nil {
		return fmt.Errorf("failed to remove siacoin elements: %w", err)
	}

	// update siacoin element proofs
	siacoinElements, err := tx.SiacoinStateElements()
	if err != nil {
		return fmt.Errorf("failed to get siacoin state elements: %w", err)
	}
	for i := range siacoinElements {
		cru.UpdateElementProof(&siacoinElements[i])
	}

	// revert siafund element changes
	if err := tx.AddSiafundElements(addedSiafundElements); err != nil {
		return fmt.Errorf("failed to add siafund elements: %w", err)
	} else if err := tx.RemoveSiafundElements(deletedSiafundElements); err != nil {
		return fmt.Errorf("failed to remove siafund elements: %w", err)
	}

	// update siafund element proofs
	siafundElements, err := tx.SiafundStateElements()
	if err != nil {
		return fmt.Errorf("failed to get siafund state elements: %w", err)
	}
	for i := range siafundElements {
		cru.UpdateElementProof(&siafundElements[i])
	}

	return tx.RevertEvents(revertedIndex)
}
