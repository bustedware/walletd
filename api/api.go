package api

import (
	"encoding/json"
	"time"

	"go.sia.tech/core/types"
	"go.sia.tech/walletd/wallet"
)

// A GatewayPeer is a currently-connected peer.
type GatewayPeer struct {
	Addr    string `json:"addr"`
	Inbound bool   `json:"inbound"`
	Version string `json:"version"`

	FirstSeen      time.Time     `json:"firstSeen"`
	ConnectedSince time.Time     `json:"connectedSince"`
	SyncedBlocks   uint64        `json:"syncedBlocks"`
	SyncDuration   time.Duration `json:"syncDuration"`
}

// TxpoolBroadcastRequest is the request type for /txpool/broadcast.
type TxpoolBroadcastRequest struct {
	Transactions   []types.Transaction   `json:"transactions"`
	V2Transactions []types.V2Transaction `json:"v2transactions"`
}

// TxpoolTransactionsResponse is the response type for /txpool/transactions.
type TxpoolTransactionsResponse struct {
	Transactions   []types.Transaction   `json:"transactions"`
	V2Transactions []types.V2Transaction `json:"v2transactions"`
}

// BalanceResponse is the response type for /wallets/:name/balance.
type BalanceResponse struct {
	Balance wallet.Balance
	ID      wallet.ID
}

// WalletReserveRequest is the request type for /wallets/:id/reserve.
type WalletReserveRequest struct {
	SiacoinOutputs []types.SiacoinOutputID `json:"siacoinOutputs"`
	SiafundOutputs []types.SiafundOutputID `json:"siafundOutputs"`
	Duration       time.Duration           `json:"duration"`
}

// A WalletUpdateRequest is a request to update a wallet
type WalletUpdateRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Metadata    json.RawMessage `json:"metadata"`
}

// WalletReleaseRequest is the request type for /wallets/:id/release.
type WalletReleaseRequest struct {
	SiacoinOutputs []types.SiacoinOutputID `json:"siacoinOutputs"`
	SiafundOutputs []types.SiafundOutputID `json:"siafundOutputs"`
}

// WalletFundRequest is the request type for /wallets/:id/fund.
type WalletFundRequest struct {
	Transaction   types.Transaction `json:"transaction"`
	Amount        types.Currency    `json:"amount"`
	ChangeAddress types.Address     `json:"changeAddress"`
}

// WalletFundSFRequest is the request type for /wallets/:id/fundsf.
type WalletFundSFRequest struct {
	Transaction   types.Transaction `json:"transaction"`
	Amount        uint64            `json:"amount"`
	ChangeAddress types.Address     `json:"changeAddress"`
	ClaimAddress  types.Address     `json:"claimAddress"`
}

// WalletFundResponse is the response type for /wallets/:id/fund.
type WalletFundResponse struct {
	Transaction types.Transaction   `json:"transaction"`
	ToSign      []types.Hash256     `json:"toSign"`
	DependsOn   []types.Transaction `json:"dependsOn"`
}

// SeedSignRequest requests that a transaction be signed using the keys derived
// from the given indices.
type SeedSignRequest struct {
	Transaction types.Transaction `json:"transaction"`
	Keys        []uint64          `json:"keys"`
}
