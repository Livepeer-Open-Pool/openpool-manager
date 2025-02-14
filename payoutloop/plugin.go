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
	log "github.com/sirupsen/logrus"
	"io/ioutil"
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
	logger            *log.Entry
}

// Ensure PayoutLoopPlugin implements PluginInterface ✅
var _ pool.PluginInterface = (*PayoutLoopPlugin)(nil)

// Init initializes the payout loop with region from the config
func (p *PayoutLoopPlugin) Init(cfg config.Config, store pool.StorageInterface) {
	threshold, ok := new(big.Int).SetString(cfg.PayoutLoopConfig.PayoutThreshold, 10)
	if !ok {
		// More structured fatal logging here
		log.WithFields(log.Fields{
			"payoutThreshold": cfg.PayoutLoopConfig.PayoutThreshold,
		}).Fatal("Failed to parse payout threshold from config")
	}

	// Create a structured logger for this plugin
	p.logger = log.WithFields(log.Fields{
		"component": "PayoutLoopPlugin",
		"region":    cfg.Region,
	})

	p.logger.Info("Initializing PayoutLoopPlugin")

	p.store = store
	p.region = cfg.Region
	p.rpcUrl = cfg.PayoutLoopConfig.RPCUrl
	p.payoutFrequency = cfg.PayoutLoopConfig.PayoutFrequencySeconds
	p.payoutThreshold = threshold
	p.keyPath = cfg.PayoutLoopConfig.PrivateKeyStorePath
	p.keyPassphrasePath = cfg.PayoutLoopConfig.PrivateKeyPassphrasePath

	p.logger.WithFields(log.Fields{
		"rpcUrl":            p.rpcUrl,
		"payoutFrequency":   p.payoutFrequency,
		"payoutThreshold":   p.payoutThreshold.String(),
		"keyPath":           p.keyPath,
		"keyPassphrasePath": p.keyPassphrasePath,
	}).Info("PayoutLoopPlugin configuration loaded")
}

// Start the payout loop
func (p *PayoutLoopPlugin) Start() {
	p.logger.WithField("payoutFrequency", p.payoutFrequency).
		Info("Payout Loop started")

	for {
		p.logger.Debug("Fetching all workers for potential payout...")

		workers, err := p.store.GetWorkers()
		if err != nil {
			p.logger.WithError(err).Error("Error fetching workers from store")
			time.Sleep(time.Duration(p.payoutFrequency) * time.Second)
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
			p.logger.WithError(err).Fatal("Failed to connect to the Ethereum client")
		}

		// Wrap the RPC client into an ethclient
		client := ethclient.NewClient(rpcClient)

		// Load the JSON keystore file.
		keyJSON, err := ioutil.ReadFile(p.keyPath)
		if err != nil {
			p.logger.WithError(err).Fatal("Failed to read keystore file")
		}

		// Decrypt the key using your keystore passphrase.
		passphrase, err := ioutil.ReadFile(p.keyPassphrasePath)
		if err != nil {
			p.logger.WithError(err).Fatal("Failed to read passphrase file")
		}

		key, err := keystore.DecryptKey(keyJSON, string(passphrase))
		if err != nil {
			p.logger.WithError(err).Fatal("Failed to decrypt keystore")
		}
		privateKey := key.PrivateKey

		for _, worker := range workers {
			//nodeType := worker.GetNodeType()
			region := worker.GetRegion()
			ethAddress := worker.GetID()
			// Assume worker.PendingFees is stored in wei as an int64.
			pendingFees := new(big.Int).SetInt64(worker.GetPendingFees())
			// Log at debug level with context
			p.logger.WithFields(log.Fields{
				"workerAddr":   ethAddress,
				"workerRegion": region,
				"pendingFees":  pendingFees.String(),
				"threshold":    p.payoutThreshold.String(),
				"cmpThreshold": pendingFees.Cmp(p.payoutThreshold),
			}).Debug("Checking if worker fees exceed threshold")

			if pendingFees.Cmp(p.payoutThreshold) >= 0 {
				payoutAmount := pendingFees
				recipient := common.HexToAddress(ethAddress)
				p.logger.WithFields(log.Fields{
					"workerAddr":   ethAddress,
					"pendingFees":  pendingFees.String(),
					"payoutAmount": payoutAmount.String(),
				}).Info("Threshold reached, initiating payout")

				// Send the payout and capture the transaction hash.
				txHash, err := SendEth(client, privateKey, payoutAmount, recipient)
				if err != nil {
					p.logger.WithFields(log.Fields{
						"workerAddr": ethAddress,
						"payoutAmt":  payoutAmount.String(),
					}).WithError(err).Error("Failed to send payout")
					continue
				}
				// Record the payout
				p.logger.WithFields(log.Fields{
					"workerAddr":   ethAddress,
					"txHash":       txHash.Hex(),
					"payoutAmount": payoutAmount.String(),
				}).Info("Payout sent, creating pool payout record")

				if err := p.store.AddPaidFees(ethAddress, payoutAmount.Int64(), txHash.Hex(), p.region, worker.GetNodeType()); err != nil {
					p.logger.WithFields(log.Fields{
						"workerAddr": ethAddress,
						"txHash":     txHash.Hex(),
					}).WithError(err).Error("Failed to create pool payout record")
				} else {
					p.logger.WithFields(log.Fields{
						"workerAddr":   ethAddress,
						"payoutAmount": payoutAmount.String(),
						"txHash":       txHash.Hex(),
					}).Info("Payout recorded successfully")
				}
			}
		}

		p.logger.Debug("Payout cycle complete, sleeping")
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
