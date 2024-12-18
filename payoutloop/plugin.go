package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"fmt"
	pool "github.com/Livepeer-Open-Pool/openpool-plugin"
	"github.com/Livepeer-Open-Pool/openpool-plugin/config"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"time"
)

type PayoutLoopPlugin struct {
	store             pool.StorageInterface
	region            string
	rpcUrl            string
	keyPath           string
	keyPassphrasePath string
	payoutThreshold   *big.Int
	payoutFrequency   int
}

// Ensure PayoutLoopPlugin implements PluginInterface ✅
var _ pool.PluginInterface = (*PayoutLoopPlugin)(nil)

// Init initializes the payout loop with region from the config
func (p *PayoutLoopPlugin) Init(cfg config.Config, store pool.StorageInterface) {
	threshold, ok := new(big.Int).SetString(cfg.PayoutLoopConfig.PayoutThreshold, 10)
	if !ok {
		//log.Printf("payloop failed to load payout threshold ")
		panic("payloop failed to load payout threshold from config")
	}
	p.store = store
	p.region = cfg.Region
	p.rpcUrl = cfg.PayoutLoopConfig.RPCUrl
	p.payoutFrequency = cfg.PayoutLoopConfig.PayoutFrequencySeconds
	p.payoutThreshold = threshold
	p.keyPath = cfg.PayoutLoopConfig.PrivateKeyStorePath
	p.keyPassphrasePath = cfg.PayoutLoopConfig.PrivateKeyPassphrasePath
}

// Start the payout loop
func (p *PayoutLoopPlugin) Start() {
	fmt.Println("Payout Loop started in region:", p.region)

	for {
		fmt.Println("Payout Loop fetching all workers...")

		workers, err := p.store.GetWorkers() // No region filter needed
		if err != nil {
			log.Printf("Error fetching workers: %v", err)
			continue
		}

		// Connect to an Ethereum node.
		customTransport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		customClient := &http.Client{Transport: customTransport}

		// Use rpc.DialOptions instead of ethclient.Dial
		rpcClient, err := rpc.DialOptions(context.Background(), p.rpcUrl, rpc.WithHTTPClient(customClient))
		if err != nil {
			log.Fatalf("Failed to connect to the Ethereum client: %v", err)
		}

		// Wrap the RPC client into an ethclient
		client := ethclient.NewClient(rpcClient)

		// Load the JSON keystore file.
		keyJSON, err := ioutil.ReadFile(p.keyPath)
		if err != nil {
			log.Fatalf("Failed to read keystore file: %v", err)
		}

		// Decrypt the key using your keystore passphrase.
		passphrase, err := ioutil.ReadFile(p.keyPassphrasePath)
		if err != nil {
			log.Fatalf("Failed to read passphrase file: %v", err)
		}

		key, err := keystore.DecryptKey(keyJSON, string(passphrase))
		if err != nil {
			log.Fatalf("Failed to decrypt keystore: %v", err)
		}
		privateKey := key.PrivateKey

		for _, worker := range workers {
			//nodeType := worker.GetNodeType()
			region := worker.GetRegion()
			ethAddress := worker.GetID()
			// Assume worker.PendingFees is stored in wei as an int64.
			pendingFees := new(big.Int).SetInt64(worker.GetPendingFees())
			payoutAmount := pendingFees

			log.Printf("checking Worker [%s] Region [%s] to see if it pendingFees[%s] passes the threshold of [%v]. [%v] ", ethAddress, region, pendingFees, p.payoutThreshold, pendingFees.Cmp(p.payoutThreshold))

			if pendingFees.Cmp(p.payoutThreshold) >= 0 {

				recipient := common.HexToAddress(ethAddress)
				log.Printf("Worker %s has pending fees %s wei; sending payout of %s wei",
					ethAddress, pendingFees.String(), payoutAmount.String())

				// Send the payout and capture the transaction hash.
				txHash, err := SendEth(client, privateKey, payoutAmount, recipient)
				if err != nil {
					log.Printf("Failed to send payout to worker %s: %v", ethAddress, err)
					continue
				}

				log.Printf("Worker %s was paid, creating new pool payout: TxHash[%s] PayoutAmount[%v]", ethAddress, txHash.Hex(), payoutAmount.Int64())

				if err := p.store.AddPaidFees(ethAddress, payoutAmount.Int64(), txHash.Hex(), p.region, worker.GetNodeType()); err != nil {
					log.Printf("Failed to create pool payout record for worker %s: %v", ethAddress, err)
				} else {
					log.Printf("Payout of %s wei sent and recorded for worker %s", payoutAmount.String(), ethAddress)
				}
			}
		}

		time.Sleep(time.Duration(p.payoutFrequency) * time.Second)
	}
}

// SendEth sends a specified amount of ETH to a recipient address and returns the transaction hash.
func SendEth(client *ethclient.Client, privateKey *ecdsa.PrivateKey, amount *big.Int, to common.Address) (common.Hash, error) {
	ctx := context.Background()

	// Derive the sender address from the private key.
	from := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Retrieve the next available nonce for the sender.
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get nonce: %v", err)
	}

	// Set a fixed gas limit.
	gasLimit := uint64(1_000_000)

	// Get the current suggested gas price.
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to suggest gas price: %v", err)
	}

	// Define the maximum acceptable gas price.
	maxGasPrice, ok := new(big.Int).SetString("5000000000000", 10)
	if !ok {
		return common.Hash{}, fmt.Errorf("invalid gas price threshold")
	}
	if gasPrice.Cmp(maxGasPrice) > 0 {
		return common.Hash{}, fmt.Errorf("gas price %v exceeds threshold %v", gasPrice, maxGasPrice)
	}

	// Create the transaction.
	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)

	// Get the network's chain ID.
	chainID, err := client.NetworkID(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get chain ID: %v", err)
	}

	// Sign the transaction using EIP-155.
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to sign transaction: %v", err)
	}

	// Send the signed transaction.
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return common.Hash{}, fmt.Errorf("failed to send transaction: %v", err)
	}

	return signedTx.Hash(), nil
}

// Exported symbol for plugin loading
var PluginInstance PayoutLoopPlugin
