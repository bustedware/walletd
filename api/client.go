package api

import (
	"encoding/json"
	"fmt"
	"time"

	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/jape"
	"go.sia.tech/walletd/wallet"
)

// A Client provides methods for interacting with a walletd API server.
type Client struct {
	c jape.Client
	n *consensus.Network // for ConsensusTipState
}

// TxpoolBroadcast broadcasts a set of transaction to the network.
func (c *Client) TxpoolBroadcast(txns []types.Transaction, v2txns []types.V2Transaction) (err error) {
	err = c.c.POST("/txpool/broadcast", TxpoolBroadcastRequest{txns, v2txns}, nil)
	return
}

// TxpoolTransactions returns all transactions in the transaction pool.
func (c *Client) TxpoolTransactions() (txns []types.Transaction, v2txns []types.V2Transaction, err error) {
	var resp TxpoolTransactionsResponse
	err = c.c.GET("/txpool/transactions", &resp)
	return resp.Transactions, resp.V2Transactions, err
}

// TxpoolFee returns the recommended fee (per weight unit) to ensure a high
// probability of inclusion in the next block.
func (c *Client) TxpoolFee() (resp types.Currency, err error) {
	err = c.c.GET("/txpool/fee", &resp)
	return
}

// ConsensusNetwork returns the node's network metadata.
func (c *Client) ConsensusNetwork() (resp *consensus.Network, err error) {
	resp = new(consensus.Network)
	err = c.c.GET("/consensus/network", resp)
	return
}

// ConsensusTip returns the current tip index.
func (c *Client) ConsensusTip() (resp types.ChainIndex, err error) {
	err = c.c.GET("/consensus/tip", &resp)
	return
}

// ConsensusTipState returns the current tip state.
func (c *Client) ConsensusTipState() (resp consensus.State, err error) {
	if c.n == nil {
		c.n, err = c.ConsensusNetwork()
		if err != nil {
			return
		}
	}
	err = c.c.GET("/consensus/tipstate", &resp)
	resp.Network = c.n
	return
}

// SyncerPeers returns the current peers of the syncer.
func (c *Client) SyncerPeers() (resp []GatewayPeer, err error) {
	err = c.c.GET("/syncer/peers", &resp)
	return
}

// SyncerConnect adds the address as a peer of the syncer.
func (c *Client) SyncerConnect(addr string) (err error) {
	err = c.c.POST("/syncer/connect", addr, nil)
	return
}

// SyncerBroadcastBlock broadcasts a block to all peers.
func (c *Client) SyncerBroadcastBlock(b types.Block) (err error) {
	err = c.c.POST("/syncer/broadcast/block", b, nil)
	return
}

// Wallets returns the set of tracked wallets.
func (c *Client) Wallets() (ws map[string]json.RawMessage, err error) {
	err = c.c.GET("/wallets", &ws)
	return
}

// AddWallet adds a wallet to the set of tracked wallets.
func (c *Client) AddWallet(uw WalletUpdateRequest) (w wallet.Wallet, err error) {
	err = c.c.POST("/wallets", uw, &w)
	return
}

// UpdateWallet updates a wallet.
func (c *Client) UpdateWallet(id wallet.ID, uw WalletUpdateRequest) (w wallet.Wallet, err error) {
	err = c.c.POST(fmt.Sprintf("/wallets/%v", id), uw, &w)
	return
}

// RemoveWallet deletes a wallet. If the wallet is currently subscribed, it will
// be unsubscribed.
func (c *Client) RemoveWallet(id wallet.ID) (err error) {
	err = c.c.DELETE(fmt.Sprintf("/wallets/%v", id))
	return
}

// Wallet returns a client for interacting with the specified wallet.
func (c *Client) Wallet(id wallet.ID) *WalletClient {
	return &WalletClient{c: c.c, id: id}
}

// Resubscribe subscribes the wallet to consensus updates, starting at the
// specified height.
func (c *Client) Resubscribe(height uint64) (err error) {
	err = c.c.POST("/resubscribe", height, nil)
	return
}

// A WalletClient provides methods for interacting with a particular wallet on a
// walletd API server.
type WalletClient struct {
	c  jape.Client
	id wallet.ID
}

// AddAddress adds the specified address and associated metadata to the
// wallet.
func (c *WalletClient) AddAddress(a wallet.Address) (err error) {
	err = c.c.PUT(fmt.Sprintf("/wallets/%v/addresses", c.id), a)
	return
}

// RemoveAddress removes the specified address from the wallet.
func (c *WalletClient) RemoveAddress(addr types.Address) (err error) {
	err = c.c.DELETE(fmt.Sprintf("/wallets/%v/addresses/%v", c.id, addr))
	return
}

// Addresses the addresses controlled by the wallet.
func (c *WalletClient) Addresses() (resp []wallet.Address, err error) {
	err = c.c.GET(fmt.Sprintf("/wallets/%v/addresses", c.id), &resp)
	return
}

// Balance returns the current wallet balance.
func (c *WalletClient) Balance() (resp BalanceResponse, err error) {
	var ret wallet.Balance
	err = c.c.GET(fmt.Sprintf("/wallets/%v/balance", c.id), &ret)
	return BalanceResponse{ID: c.id, Balance: ret}, err
}

// Events returns all events relevant to the wallet.
func (c *WalletClient) Events(offset, limit int) (resp []wallet.Event, err error) {
	err = c.c.GET(fmt.Sprintf("/wallets/%v/events?offset=%d&limit=%d", c.id, offset, limit), &resp)
	return
}

// PoolTransactions returns all txpool transactions relevant to the wallet.
func (c *WalletClient) PoolTransactions() (resp []wallet.PoolTransaction, err error) {
	err = c.c.GET(fmt.Sprintf("/wallets/%v/txpool", c.id), &resp)
	return
}

// SiacoinOutputs returns the set of unspent outputs controlled by the wallet.
func (c *WalletClient) SiacoinOutputs(offset, limit int) (sc []types.SiacoinElement, err error) {
	err = c.c.GET(fmt.Sprintf("/wallets/%v/outputs/siacoin?offset=%d&limit=%d", c.id, offset, limit), &sc)
	return
}

// SiafundOutputs returns the set of unspent outputs controlled by the wallet.
func (c *WalletClient) SiafundOutputs(offset, limit int) (sf []types.SiafundElement, err error) {
	err = c.c.GET(fmt.Sprintf("/wallets/%v/outputs/siafund?offset=%d&limit=%d", c.id, offset, limit), &sf)
	return
}

// Reserve reserves a set outputs for use in a transaction.
func (c *WalletClient) Reserve(sc []types.SiacoinOutputID, sf []types.SiafundOutputID, duration time.Duration) (err error) {
	err = c.c.POST(fmt.Sprintf("/wallets/%v/reserve", c.id), WalletReserveRequest{
		SiacoinOutputs: sc,
		SiafundOutputs: sf,
		Duration:       duration,
	}, nil)
	return
}

// Release releases a set of previously-reserved outputs.
func (c *WalletClient) Release(sc []types.SiacoinOutputID, sf []types.SiafundOutputID) (err error) {
	err = c.c.POST(fmt.Sprintf("/wallets/%v/release", c.id), WalletReleaseRequest{
		SiacoinOutputs: sc,
		SiafundOutputs: sf,
	}, nil)
	return
}

// Fund funds a siacoin transaction.
func (c *WalletClient) Fund(txn types.Transaction, amount types.Currency, changeAddr types.Address) (resp WalletFundResponse, err error) {
	err = c.c.POST(fmt.Sprintf("/wallets/%v/fund", c.id), WalletFundRequest{
		Transaction:   txn,
		Amount:        amount,
		ChangeAddress: changeAddr,
	}, &resp)
	return
}

// FundSF funds a siafund transaction.
func (c *WalletClient) FundSF(txn types.Transaction, amount uint64, changeAddr, claimAddr types.Address) (resp WalletFundResponse, err error) {
	err = c.c.POST(fmt.Sprintf("/wallets/%v/fundsf", c.id), WalletFundSFRequest{
		Transaction:   txn,
		Amount:        amount,
		ChangeAddress: changeAddr,
		ClaimAddress:  claimAddr,
	}, &resp)
	return
}

// NewClient returns a client that communicates with a walletd server listening
// on the specified address.
func NewClient(addr, password string) *Client {
	return &Client{c: jape.Client{
		BaseURL:  addr,
		Password: password,
	}}
}
