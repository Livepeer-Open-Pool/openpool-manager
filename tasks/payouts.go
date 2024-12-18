package tasks

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/config"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"io/ioutil"
	"log"
	"math/big"
	"time"
)

func StartPayoutLoop(dbStorage *storage.Storage, cfg *config.PayoutLoopConfig) {
	// Poll every cfg.PayoutFrequencySeconds seconds.
	interval := time.Duration(cfg.PayoutFrequencySeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Define the threshold: 0.005 ETH in wei (0.005 * 1e18 = 5000000000000000).
	threshold, ok := new(big.Int).SetString(cfg.PayoutThreshold, 10)
	if !ok {
		log.Println("failed to parse threshold")
		return
	}

	for range ticker.C {
		log.Println("Checking remote workers for payouts...")
		// Retrieve all remote workers.
		workers, err := dbStorage.RemoteWorkerRepo.FindAll()
		if err != nil {
			log.Fatalf("failed to fetch remote workers: %v", err)
		}
		// Payout amount is exactly 0.001 ETH.
		payoutAmount := threshold
		// Connect to an Ethereum node.
		client, err := ethclient.Dial(cfg.RPCUrl)
		if err != nil {
			log.Fatalf("Failed to connect to the Ethereum client: %v", err)
		}

		// Load the JSON keystore file.
		keyJSON, err := ioutil.ReadFile(cfg.PrivateKeyStorePath)
		if err != nil {
			log.Fatalf("Failed to read keystore file: %v", err)
		}

		// Decrypt the key using your keystore passphrase.
		passphrase, err := ioutil.ReadFile(cfg.PrivateKeyPassphrasePath)
		if err != nil {
			log.Fatalf("Failed to read passphrase file: %v", err)
		}

		key, err := keystore.DecryptKey(keyJSON, string(passphrase))
		if err != nil {
			log.Fatalf("Failed to decrypt keystore: %v", err)
		}
		privateKey := key.PrivateKey

		// Iterate over each worker.
		for _, worker := range workers {
			// Assume worker.PendingFees is stored in wei as an int64.
			pendingFees := new(big.Int).SetInt64(worker.PendingFees)
			log.Printf("checking Worker [%s] Region [%s] to see if it pendingFees[%s] passes the threshold of [%v]. [%v] ", worker.EthAddress, worker.Region, pendingFees, threshold, pendingFees.Cmp(threshold))

			if pendingFees.Cmp(threshold) >= 0 {
				recipient := common.HexToAddress(worker.EthAddress)
				log.Printf("Worker %s has pending fees %s wei; sending payout of %s wei",
					worker.EthAddress, pendingFees.String(), payoutAmount.String())

				// Send the payout and capture the transaction hash.
				txHash, err := SendEth(client, privateKey, payoutAmount, recipient)
				if err != nil {
					log.Printf("Failed to send payout to worker %s: %v", worker.EthAddress, err)
					continue
				}

				log.Printf("Worker %s was paid, creating new pool payout: TxHash[%s] PayoutAmount[%v]", worker.EthAddress, txHash, payoutAmount.Int64())
				// Create the pool payout record.
				payout := &models.PoolPayout{
					EthAddress: worker.EthAddress,
					TxHash:     txHash.Hex(),
					Fees:       payoutAmount.Int64(),
				}
				if err := dbStorage.PoolPayoutRepository.Create(payout); err != nil {
					log.Printf("Failed to create pool payout record for worker %s: %v", worker.EthAddress, err)
				} else {
					log.Printf("Payout of %s wei sent and recorded for worker %s", payoutAmount.String(), worker.EthAddress)
				}

				if err := dbStorage.RemoteWorkerRepo.AddPaidFees(worker.EthAddress, worker.NodeType, worker.Region, payoutAmount.Int64(), worker.EndpointHash); err != nil {
					log.Printf("Failed to update paid fees record for worker %s: %v", worker.EthAddress, err)
				} else {
					log.Printf("Paid Fees [%v] recorded for worker %s", payoutAmount.String(), worker.EthAddress)
				}
			}
		}
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
