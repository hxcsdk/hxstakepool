// dcrclient.go
package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
	"github.com/decred/dcrrpcclient"
	"github.com/decred/dcrutil"
)

// functionName
type functionName int

const (
	getNewAddressFn functionName = iota
	validateAddressFn
	createMultisigFn
	importScriptFn
	ticketsForAddressFn
	getTicketVoteBitsFn
	getTicketsVoteBitsFn
	setTicketVoteBitsFn
	getTxOutFn
	getStakeInfoFn
	pingFn
)

var (
	// cacheTimerStakeInfo is the duration of time after which to
	// access the wallet and update the stake information instead
	// of returning cached stake information.
	cacheTimerStakeInfo = 5 * time.Minute

	// cacheTimerGetTickets is the duration of time after which to
	// access the wallet and update the ticket list for an address
	// instead of returning cached ticket information.
	cacheTimerGetTickets = 20 * time.Second

	// allowTimerSetVoteBits is the cooldown timer between set vote
	// bits calls for some given ticket. After this time, the vote
	// bits may be set again.
	allowTimerSetVoteBits = 30 * time.Second
)

var (
	ErrSetVoteBitsCoolDown = fmt.Errorf("can not set the vote bits because " +
		"last call was too soon")
)

// calcNextReqDifficultyResponse
type getNewAddressResponse struct {
	address dcrutil.Address
	err     error
}

// getNewAddressMsg
type getNewAddressMsg struct {
	reply chan getNewAddressResponse
}

// validateAddressResponse
type validateAddressResponse struct {
	addrInfo *dcrjson.ValidateAddressWalletResult
	err      error
}

// validateAddressMsg
type validateAddressMsg struct {
	address dcrutil.Address
	reply   chan validateAddressResponse
}

// createMultisigResponse
type createMultisigResponse struct {
	multisigInfo *dcrjson.CreateMultiSigResult
	err          error
}

// createMultisigMsg
type createMultisigMsg struct {
	required  int
	addresses []dcrutil.Address
	reply     chan createMultisigResponse
}

// importScriptResponse
type importScriptResponse struct {
	err error
}

// importScriptMsg
type importScriptMsg struct {
	script []byte
	reply  chan importScriptResponse
}

// ticketsForAddressResponse
type ticketsForAddressResponse struct {
	tickets *dcrjson.TicketsForAddressResult
	err     error
}

// ticketsForAddressMsg
type ticketsForAddressMsg struct {
	address dcrutil.Address
	reply   chan ticketsForAddressResponse
}

// getTicketVoteBitsResponse
type getTicketVoteBitsResponse struct {
	voteBits *dcrjson.GetTicketVoteBitsResult
	err      error
}

// getTicketVoteBitsMsg
type getTicketVoteBitsMsg struct {
	hash  *chainhash.Hash
	reply chan getTicketVoteBitsResponse
}

// getTicketsVoteBitsResponse
type getTicketsVoteBitsResponse struct {
	voteBitsList *dcrjson.GetTicketsVoteBitsResult
	err          error
}

// getTicketsVoteBitsMsg
type getTicketsVoteBitsMsg struct {
	hashes []*chainhash.Hash
	reply  chan getTicketsVoteBitsResponse
}

// setTicketVoteBitsResponse
type setTicketVoteBitsResponse struct {
	err error
}

// setTicketVoteBitsMsg
type setTicketVoteBitsMsg struct {
	hash     *chainhash.Hash
	voteBits uint16
	reply    chan setTicketVoteBitsResponse
}

// getTxOutResponse
type getTxOutResponse struct {
	txOut *dcrjson.GetTxOutResult
	err   error
}

// getTxOutMsg
type getTxOutMsg struct {
	hash  *chainhash.Hash
	idx   uint32
	reply chan getTxOutResponse
}

// getStakeInfoResponse
type getStakeInfoResponse struct {
	stakeInfo *dcrjson.GetStakeInfoResult
	err       error
}

// getStakeInfoMsg
type getStakeInfoMsg struct {
	reply chan getStakeInfoResponse
}

// pingResponse
type pingResponse struct {
	err error
}

// pingMsg
type pingMsg struct {
	reply chan pingResponse
}

// walletRPCHandler
func (w *walletSvrManager) walletRPCHandler() {
out:
	for {
		select {
		case m := <-w.msgChan:
			switch msg := m.(type) {
			case getNewAddressMsg:
				resp := w.executeInSequence(getNewAddressFn, msg)
				respTyped := resp.(*getNewAddressResponse)
				msg.reply <- *respTyped
			case validateAddressMsg:
				resp := w.executeInSequence(validateAddressFn, msg)
				respTyped := resp.(*validateAddressResponse)
				msg.reply <- *respTyped
			case createMultisigMsg:
				resp := w.executeInSequence(createMultisigFn, msg)
				respTyped := resp.(*createMultisigResponse)
				msg.reply <- *respTyped
			case importScriptMsg:
				resp := w.executeInSequence(importScriptFn, msg)
				respTyped := resp.(*importScriptResponse)
				msg.reply <- *respTyped
			case ticketsForAddressMsg:
				resp := w.executeInSequence(ticketsForAddressFn, msg)
				respTyped := resp.(*ticketsForAddressResponse)
				msg.reply <- *respTyped
			case getTicketVoteBitsMsg:
				resp := w.executeInSequence(getTicketVoteBitsFn, msg)
				respTyped := resp.(*getTicketVoteBitsResponse)
				msg.reply <- *respTyped
			case getTicketsVoteBitsMsg:
				resp := w.executeInSequence(getTicketsVoteBitsFn, msg)
				respTyped := resp.(*getTicketsVoteBitsResponse)
				msg.reply <- *respTyped
			case setTicketVoteBitsMsg:
				resp := w.executeInSequence(setTicketVoteBitsFn, msg)
				respTyped := resp.(*setTicketVoteBitsResponse)
				msg.reply <- *respTyped
			case getTxOutMsg:
				resp := w.executeInSequence(getTxOutFn, msg)
				respTyped := resp.(*getTxOutResponse)
				msg.reply <- *respTyped
			case getStakeInfoMsg:
				resp := w.executeInSequence(getStakeInfoFn, msg)
				respTyped := resp.(*getStakeInfoResponse)
				msg.reply <- *respTyped
			case pingMsg:
				resp := w.executeInSequence(pingFn, msg)
				respTyped := resp.(*pingResponse)
				msg.reply <- *respTyped
			default:
				log.Infof("Invalid message type in wallet RPC "+
					"handler: %T", msg)
			}

		case <-w.quit:
			break out
		}
	}

	w.wg.Done()
	log.Infof("Wallet RPC handler done")
}

// executeInSequence
func (w *walletSvrManager) executeInSequence(fn functionName, msg interface{}) interface{} {
	switch fn {
	case getNewAddressFn:
		resp := new(getNewAddressResponse)
		addrs := make([]dcrutil.Address, w.serversLen, w.serversLen)
		for i, s := range w.servers {
			addr, err := s.GetNewAddress("default")
			if err != nil {
				log.Infof("getNewAddressFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			addrs[i] = addr
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if !bytes.Equal(addrs[i].ScriptAddress(),
				addrs[i+1].ScriptAddress()) {
				log.Infof("getNewAddressFn nonequiv failure on servers "+
					"%v, %v (%v != %v)", i, i+1, addrs[i].ScriptAddress(), addrs[i+1].ScriptAddress())
				resp.err = fmt.Errorf("non equivalent address returned")
				return resp
			}
		}

		resp.address = addrs[0]
		return resp

	case validateAddressFn:
		vam := msg.(validateAddressMsg)
		resp := new(validateAddressResponse)
		vawrs := make([]*dcrjson.ValidateAddressWalletResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			vawr, err := s.ValidateAddress(vam.address)
			if err != nil {
				log.Infof("validateAddressFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			vawrs[i] = vawr
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if vawrs[i].PubKey != vawrs[i+1].PubKey {
				log.Infof("validateAddressFn nonequiv failure on servers "+
					"%v, %v (%v != %v)", i, i+1, vawrs[i].PubKey, vawrs[i+1].PubKey)
				resp.err = fmt.Errorf("non equivalent pubkey returned")
				return resp
			}
		}

		resp.addrInfo = vawrs[0]
		return resp

	case createMultisigFn:
		cmsm := msg.(createMultisigMsg)
		resp := new(createMultisigResponse)
		cmsrs := make([]*dcrjson.CreateMultiSigResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			cmsr, err := s.CreateMultisig(cmsm.required, cmsm.addresses)
			if err != nil {
				log.Infof("createMultisigFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			cmsrs[i] = cmsr
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if cmsrs[i].RedeemScript != cmsrs[i+1].RedeemScript {
				log.Infof("createMultisigFn nonequiv failure on servers "+
					"%v, %v (%v != %v)", i, i+1, cmsrs[i].RedeemScript, cmsrs[i+1].RedeemScript)
				resp.err = fmt.Errorf("non equivalent redeem script returned")
				return resp
			}
		}

		resp.multisigInfo = cmsrs[0]
		return resp

	case importScriptFn:
		ism := msg.(importScriptMsg)
		resp := new(importScriptResponse)
		isErrors := make([]error, w.serversLen, w.serversLen)
		for i, s := range w.servers {
			err := s.ImportScript(ism.script)
			isErrors[i] = err
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}

			notIsNil1 := isErrors[i] != nil
			notIsNil2 := isErrors[i+1] != nil
			if notIsNil1 != notIsNil2 {
				log.Infof("importScriptFn nonequiv failure 1 on servers %v, %v",
					i, i+1)
				resp.err = fmt.Errorf("non equivalent error returned 1")
				return resp
			}

			if notIsNil1 && notIsNil2 {
				if isErrors[i].Error() != isErrors[i+1].Error() {
					log.Infof("importScriptFn nonequiv failure 2 on  "+
						"servers %v, %v", i, i+1)
					resp.err = fmt.Errorf("non equivalent error returned 2")
					return resp
				}
			}
		}

		resp.err = isErrors[0]
		return resp

	case ticketsForAddressFn:
		tfam := msg.(ticketsForAddressMsg)
		resp := new(ticketsForAddressResponse)
		tfars := make([]*dcrjson.TicketsForAddressResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			tfar, err := s.TicketsForAddress(tfam.address)
			if err != nil {
				log.Infof("ticketsForAddressFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			tfars[i] = tfar
		}

		resp.tickets = tfars[0]
		return resp

	case getTicketVoteBitsFn:
		gtvbm := msg.(getTicketVoteBitsMsg)
		resp := new(getTicketVoteBitsResponse)
		gtvbrs := make([]*dcrjson.GetTicketVoteBitsResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			gtvbr, err := s.GetTicketVoteBits(gtvbm.hash)
			if err != nil {
				log.Infof("getTicketVoteBitsFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			gtvbrs[i] = gtvbr
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if gtvbrs[i].VoteBits != gtvbrs[i+1].VoteBits {
				log.Infof("getTicketVoteBitsFn nonequiv failure on servers "+
					"%v, %v", i, i+1)
				resp.err = fmt.Errorf("non equivalent votebits returned")
				return resp
			}
		}

		resp.voteBits = gtvbrs[0]
		return resp

	case getTicketsVoteBitsFn:
		gtvbm := msg.(getTicketsVoteBitsMsg)
		resp := new(getTicketsVoteBitsResponse)
		gtvbrs := make([]*dcrjson.GetTicketsVoteBitsResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			gtvbr, err := s.GetTicketsVoteBits(gtvbm.hashes)
			if err != nil {
				log.Infof("getTicketsVoteBitsFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			gtvbrs[i] = gtvbr
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if len(gtvbrs[i].VoteBitsList) == 0 ||
				len(gtvbrs[i+1].VoteBitsList) == 0 {
				if len(gtvbrs[i].VoteBitsList) != len(gtvbrs[i+1].VoteBitsList) {
					log.Infof("getTicketsVoteBitsFn nonequiv failure on servers "+
						"%v, %v", i, i+1)
					resp.err = fmt.Errorf("non equivalent num elements returned")
					return resp
				}
				resp.voteBitsList = gtvbrs[0]
				return resp
			}
			nonEquiv := false
			for j := range gtvbrs[i].VoteBitsList {
				if gtvbrs[i].VoteBitsList[j].VoteBits !=
					gtvbrs[i+1].VoteBitsList[j].VoteBits {
					log.Infof("getTicketsVoteBitsFn nonequiv failure on servers "+
						"%v, %v", i, i+1)
					log.Infof("votebits for server %v is %v, server %v is %v",
						i, gtvbrs[i].VoteBitsList[j].VoteBits, i+1,
						gtvbrs[i+1].VoteBitsList[j].VoteBits)
					log.Infof("failing ticket hash: %v", gtvbm.hashes[j])
					nonEquiv = true
				}
			}
			if nonEquiv {
				resp.err = fmt.Errorf("non equivalent votebits returned")
				return resp
			}
		}

		resp.voteBitsList = gtvbrs[0]
		return resp

	case setTicketVoteBitsFn:
		stvbm := msg.(setTicketVoteBitsMsg)
		resp := new(setTicketVoteBitsResponse)
		for i, s := range w.servers {
			err := s.SetTicketVoteBits(stvbm.hash, stvbm.voteBits)
			if err != nil {
				log.Infof("setTicketVoteBitsFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
		}

		return resp

	case getTxOutFn:
		gtom := msg.(getTxOutMsg)
		resp := new(getTxOutResponse)
		gtors := make([]*dcrjson.GetTxOutResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			gtor, err := s.GetTxOut(gtom.hash, gtom.idx, true)
			if err != nil {
				log.Infof("getTxOutFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			gtors[i] = gtor
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if gtors[i].ScriptPubKey.Hex != gtors[i+1].ScriptPubKey.Hex {
				log.Infof("getTxOutFn nonequiv failure on servers "+
					"%v, %v", i, i+1)
				resp.err = fmt.Errorf("non equivalent ScriptPubKey returned")
				return resp
			}
		}

		resp.txOut = gtors[0]
		return resp

	case getStakeInfoFn:
		resp := new(getStakeInfoResponse)
		gsirs := make([]*dcrjson.GetStakeInfoResult, w.serversLen,
			w.serversLen)
		for i, s := range w.servers {
			gsir, err := s.GetStakeInfo()
			if err != nil {
				log.Infof("getStakeInfoFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			gsirs[i] = gsir
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if gsirs[i].Live != gsirs[i+1].Live {
				log.Infof("getStakeInfoFn nonequiv failure on servers "+
					"%v, %v", i, i+1)
				resp.err = fmt.Errorf("non equivalent Live returned")
				return resp
			}
		}

		resp.stakeInfo = gsirs[0]
		return resp

	case pingFn:
		resp := new(pingResponse)
		prs := make([]error, w.serversLen, w.serversLen)
		for i, s := range w.servers {
			err := s.Ping()
			if err != nil {
				log.Infof("getStakeInfoFn failure on server %v: %v", i, err)
				resp.err = err
				return resp
			}
			prs[i] = err
		}

		for i := 0; i < w.serversLen; i++ {
			if i == w.serversLen-1 {
				break
			}
			if prs[i] != prs[i+1] {
				log.Infof("pingFn nonequiv failure on servers "+
					"%v, %v", i, i+1)
				resp.err = fmt.Errorf("non equivalent error returned: %v and %v",
					prs[i], prs[i+1])
				return resp
			}
		}

		resp.err = prs[0]
		return resp
	}

	return nil
}

// ping pings all the servers and makes sure they're online. This should be
// performed before doing a write.
func (w *walletSvrManager) ping() error {
	reply := make(chan pingResponse)
	w.msgChan <- pingMsg{
		reply: reply,
	}
	response := <-reply
	return response.err
}

// GetNewAddress
//
// This should return equivalent results from all wallet RPCs. If this encounters
// a failure, it should be considered fatal.
func (w *walletSvrManager) GetNewAddress() (dcrutil.Address, error) {
	// Assert that all servers are online.
	err := w.ping()
	if err != nil {
		return nil, err
	}

	reply := make(chan getNewAddressResponse)
	w.msgChan <- getNewAddressMsg{reply: reply}
	response := <-reply
	return response.address, response.err
}

// ValidateAddress
//
// This should return equivalent results from all wallet RPCs. If this encounters
// a failure, it should be considered fatal.
func (w *walletSvrManager) ValidateAddress(addr dcrutil.Address) (*dcrjson.ValidateAddressWalletResult, error) {
	// Assert that all servers are online.
	err := w.ping()
	if err != nil {
		return nil, err
	}

	reply := make(chan validateAddressResponse)
	w.msgChan <- validateAddressMsg{
		address: addr,
		reply:   reply,
	}
	response := <-reply
	return response.addrInfo, response.err
}

// CreateMultisig
//
// This should return equivalent results from all wallet RPCs. If this encounters
// a failure, it should be considered fatal.
func (w *walletSvrManager) CreateMultisig(nreq int, addrs []dcrutil.Address) (*dcrjson.CreateMultiSigResult, error) {
	// Assert that all servers are online.
	err := w.ping()
	if err != nil {
		return nil, err
	}

	reply := make(chan createMultisigResponse)
	w.msgChan <- createMultisigMsg{
		required:  nreq,
		addresses: addrs,
		reply:     reply,
	}
	response := <-reply
	return response.multisigInfo, response.err
}

// ImportScript
//
// This should return equivalent results from all wallet RPCs. If this encounters
// a failure, it should be considered fatal.
func (w *walletSvrManager) ImportScript(script []byte) error {
	// Assert that all servers are online.
	err := w.ping()
	if err != nil {
		return err
	}

	reply := make(chan importScriptResponse)
	w.msgChan <- importScriptMsg{
		script: script,
		reply:  reply,
	}
	response := <-reply
	return response.err
}

// TicketsForAddress
//
// This can race depending on what wallet is currently processing, so failures
// from this function should NOT cause fatal errors on the web server like the
// other RPC client calls.
func (w *walletSvrManager) TicketsForAddress(address dcrutil.Address) (*dcrjson.TicketsForAddressResult, error) {
	w.cachedGetTicketsMutex.Lock()
	defer w.cachedGetTicketsMutex.Unlock()

	// See if we already have a cached copy of this information.
	// If it isn't too old, return that instead.
	cachedResp, ok := w.cachedGetTicketsMap[address.EncodeAddress()]
	if ok {
		if time.Now().Sub(cachedResp.timer) < cacheTimerGetTickets {
			return cachedResp.res, nil
		}
	}

	reply := make(chan ticketsForAddressResponse)
	w.msgChan <- ticketsForAddressMsg{
		address: address,
		reply:   reply,
	}
	response := <-reply

	// If there was no error, cache the response now.
	if response.err != nil {
		w.cachedGetTicketsMap[address.EncodeAddress()] =
			NewGetTicketsCacheData(response.tickets)
	}

	return response.tickets, response.err
}

// GetTicketVoteBits
//
// This can race depending on what wallet is currently processing, so failures
// from this function should NOT cause fatal errors on the web server like the
// other RPC client calls.
func (w *walletSvrManager) GetTicketVoteBits(hash *chainhash.Hash) (*dcrjson.GetTicketVoteBitsResult, error) {
	reply := make(chan getTicketVoteBitsResponse)
	w.msgChan <- getTicketVoteBitsMsg{
		hash:  hash,
		reply: reply,
	}
	response := <-reply
	return response.voteBits, response.err
}

// GetTicketsVoteBits
//
// This can race depending on what wallet is currently processing, so failures
// from this function should NOT cause fatal errors on the web server like the
// other RPC client calls.
func (w *walletSvrManager) GetTicketsVoteBits(hashes []*chainhash.Hash) (*dcrjson.GetTicketsVoteBitsResult, error) {
	reply := make(chan getTicketsVoteBitsResponse)
	w.msgChan <- getTicketsVoteBitsMsg{
		hashes: hashes,
		reply:  reply,
	}
	response := <-reply
	return response.voteBitsList, response.err
}

// SetTicketVoteBits
//
// This should return equivalent results from all wallet RPCs. If this encounters
// a failure, it should be considered fatal.
func (w *walletSvrManager) SetTicketVoteBits(hash *chainhash.Hash, voteBits uint16) error {
	// Assert that all servers are online.
	err := w.ping()
	if err != nil {
		return err
	}

	w.setVoteBitsCoolDownMutex.Lock()
	defer w.setVoteBitsCoolDownMutex.Unlock()

	// Throttle how often the user is allowed to change their stake
	// vote bits.
	vbSetTime, ok := w.setVoteBitsCoolDownMap[*hash]
	if ok {
		if time.Now().Sub(vbSetTime) < allowTimerSetVoteBits {
			return ErrSetVoteBitsCoolDown
		}
	}

	reply := make(chan setTicketVoteBitsResponse)
	w.msgChan <- setTicketVoteBitsMsg{
		hash:     hash,
		voteBits: voteBits,
		reply:    reply,
	}

	// If the set was successful, reset the timer.
	w.setVoteBitsCoolDownMap[*hash] = time.Now()

	response := <-reply
	return response.err
}

// GetTxOut gets a txOut status given a hash and an output index. It returns
// nothing if the output is spent, and a standard response if it is unspent.
//
// This can race depending on what wallet is currently processing, so failures
// from this function should NOT cause fatal errors on the web server like the
// other RPC client calls.
func (w *walletSvrManager) GetTxOut(hash *chainhash.Hash, idx uint32) (*dcrjson.GetTxOutResult, error) {
	reply := make(chan getTxOutResponse)
	w.msgChan <- getTxOutMsg{
		hash:  hash,
		idx:   idx,
		reply: reply,
	}
	response := <-reply
	return response.txOut, response.err
}

// getStakeInfo returns the cached current stake statistics about the wallet if
// it has been less than five minutes. If it has been longer than five minutes,
// a new request for stake information is piped through the RPC client handler
// and then cached for future reuse.
//
// This can race depending on what wallet is currently processing, so failures
// from this function should NOT cause fatal errors on the web server like the
// other RPC client calls.
func (w *walletSvrManager) getStakeInfo() (*dcrjson.GetStakeInfoResult, error) {
	// Less than five minutes has elapsed since the last call. Return
	// the previously cached stake information.
	if time.Now().Sub(w.cachedStakeInfoTimer) < cacheTimerStakeInfo {
		return w.cachedStakeInfo, nil
	}

	// Five minutes or more has passed since the last call, so request new
	// stake information.
	reply := make(chan getStakeInfoResponse)
	w.msgChan <- getStakeInfoMsg{
		reply: reply,
	}
	response := <-reply

	// If there was an error, return the error and do not reset
	// the timer.
	if response.err != nil {
		return nil, response.err
	}

	// Cache the response for future use and reset the timer.
	w.cachedStakeInfo = response.stakeInfo
	w.cachedStakeInfoTimer = time.Now()

	return response.stakeInfo, nil
}

// GetStakeInfo is the concurrency safe, exported version of getStakeInfo.
func (w *walletSvrManager) GetStakeInfo() (*dcrjson.GetStakeInfoResult, error) {
	w.cachedStakeInfoMutex.Lock()
	defer w.cachedStakeInfoMutex.Unlock()

	return w.getStakeInfo()
}

// getTicketsCacheData is a TicketsForAddressResult that also contains a time
// at which TicketsForAddress was last called. The results should only update
type getTicketsCacheData struct {
	res   *dcrjson.TicketsForAddressResult
	timer time.Time
}

func NewGetTicketsCacheData(tfar *dcrjson.TicketsForAddressResult) *getTicketsCacheData {
	return &getTicketsCacheData{tfar, time.Now()}
}

// walletSvrManager provides a concurrency safe RPC call manager for handling all
// incoming wallet server requests.
type walletSvrManager struct {
	servers    []*dcrrpcclient.Client
	serversLen int

	// cachedStakeInfo is cached information about the stake pool wallet.
	// This is required because of the time it takes to compute the
	// stake information. The included timer is used so that new stake
	// information is only queried for if 5 minutes or more has passed.
	// The mutex is used to allow concurrent access to the stake
	// information if less than five minutes has passed.
	cachedStakeInfo      *dcrjson.GetStakeInfoResult
	cachedStakeInfoTimer time.Time
	cachedStakeInfoMutex sync.Mutex

	// cachedGetTicketsMap caches TicketsForAddress responses and
	// is used to only provide new calls to the wallet RPC after a
	// cooldown period to prevent DoS attacks.
	cachedGetTicketsMap   map[string]*getTicketsCacheData
	cachedGetTicketsMutex sync.Mutex

	// setVoteBitsCoolDownMap is a map that tracks the last calls to
	// setting the votebits for a transaction. It applies a cooldown
	// so that the RPC call isn't abused.
	setVoteBitsCoolDownMap   map[chainhash.Hash]time.Time
	setVoteBitsCoolDownMutex sync.Mutex

	started  int32
	shutdown int32
	msgChan  chan interface{}
	wg       sync.WaitGroup
	quit     chan struct{}
}

// Start begins the core block handler which processes block and inv messages.
func (w *walletSvrManager) Start() {
	// Already started?
	if atomic.AddInt32(&w.started, 1) != 1 {
		return
	}

	log.Info("Starting wallet RPC manager")
	w.wg.Add(1)
	go w.walletRPCHandler()
}

// Stop gracefully shuts down the block manager by stopping all asynchronous
// handlers and waiting for them to finish.
func (w *walletSvrManager) Stop() error {
	if atomic.AddInt32(&w.shutdown, 1) != 1 {
		log.Info("Wallet RPC manager is already in the process of " +
			"shutting down")
		return nil
	}

	log.Info("Wallet RPC manager shutting down")
	close(w.quit)
	w.wg.Wait()
	return nil
}

// IsStopped
func (w *walletSvrManager) IsStopped() bool {
	return w.shutdown == 1
}

// newWalletSvrManager returns a new decred wallet server manager.
// Use Start to begin processing asynchronous block and inv updates.
func newWalletSvrManager() (*walletSvrManager, error) {
	var serverCfgs []ServerCfg
	err := json.Unmarshal(serverPoolCfg, &serverCfgs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config json: %v", err.Error())
	}

	localServers := make([]*dcrrpcclient.Client, len(serverCfgs), len(serverCfgs))
	for i, scfg := range serverCfgs {
		certs, err := ioutil.ReadFile(scfg.Cert)
		if err != nil {
			log.Errorf("Error %v", err)
		}
		connCfg := &dcrrpcclient.ConnConfig{
			Host:         scfg.Host,
			Endpoint:     "ws",
			User:         scfg.User,
			Pass:         scfg.Pass,
			Certificates: certs,
		}

		client, err := dcrrpcclient.New(connCfg, nil)
		if err != nil {
			fmt.Printf("couldn't connect to RPC server #%v: %v", scfg.Host, err)
			log.Infof("couldn't connect to RPC server #%v: %v", scfg.Host, err)
			return nil, fmt.Errorf("RPC server connection failure on start")
		}
		localServers[i] = client
	}

	wsm := walletSvrManager{
		servers:                localServers,
		serversLen:             len(localServers),
		cachedGetTicketsMap:    make(map[string]*getTicketsCacheData),
		setVoteBitsCoolDownMap: make(map[chainhash.Hash]time.Time),
		msgChan:                make(chan interface{}, 500),
		quit:                   make(chan struct{}),
	}

	// Set the timer to automatically require a new set of stake information
	// on startup.
	wsm.cachedStakeInfoTimer = time.Now().Add(-cacheTimerStakeInfo)

	return &wsm, nil
}