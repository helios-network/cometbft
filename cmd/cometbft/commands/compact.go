package commands

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/consensus"
	"github.com/cometbft/cometbft/libs/log"
	sm "github.com/cometbft/cometbft/state"
	"github.com/cometbft/cometbft/store"
)

var CompactGoLevelDBCmd = &cobra.Command{
	Use:     "experimental-compact-goleveldb",
	Aliases: []string{"experimental_compact_goleveldb"},
	Short:   "force compacts the CometBFT storage engine (only GoLevelDB supported)",
	Long: `
This is a temporary utility command that performs a force compaction on the state 
and blockstores to reduce disk space for a pruning node. This should only be run 
once the node has stopped. This command will likely be omitted in the future after
the planned refactor to the storage engine.

Currently, only GoLevelDB is supported.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.DBBackend != "goleveldb" {
			return errors.New("compaction is currently only supported with goleveldb")
		}

		compactGoLevelDBs(config.RootDir, logger)
		return nil
	},
}

func NewCompactGoLevelDBCmd(rootCmd *cobra.Command, hidden bool, defaultNodeHome string) *cobra.Command {
	flagZsh := "zsh"
	cmd := &cobra.Command{
		Use:   "experimental-compact-goleveldb",
		Short: "force compacts the CometBFT storage engine (only GoLevelDB supported)",
		Long: `This is a temporary utility command that performs a force compaction on the state 
and blockstores to reduce disk space for a pruning node. This should only be run 
once the node has stopped. This command will likely be omitted in the future after
the planned refactor to the storage engine.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			compactGoLevelDBs(defaultNodeHome, logger)
			return nil
		},
		Hidden: hidden,
		Args:   cobra.NoArgs,
	}

	cmd.Flags().Bool(flagZsh, false, "Generate Zsh completion script")

	return cmd
}

func compactGoLevelDBs(rootDir string, logger log.Logger) {
	dbNames := []string{"state", "blockstore"}
	o := &opt.Options{
		DisableSeeksCompaction: true,
	}
	wg := sync.WaitGroup{}

	for _, dbName := range dbNames {
		dbName := dbName
		wg.Add(1)
		go func() {
			defer wg.Done()
			dbPath := filepath.Join(rootDir, "data", dbName+".db")
			store, err := leveldb.OpenFile(dbPath, o)
			if err != nil {
				logger.Error("failed to initialize cometbft db", "path", dbPath, "err", err)
				return
			}
			defer store.Close()

			logger.Info("starting compaction...", "db", dbPath)

			err = store.CompactRange(util.Range{Start: nil, Limit: nil})
			if err != nil {
				logger.Error("failed to compact cometbft db", "path", dbPath, "err", err)
			}
		}()
	}
	wg.Wait()
}

var PruneBlockByBlockCmd = &cobra.Command{
	Use:   "prune-block-by-block",
	Short: "prune blocks one by one to reduce MANIFEST growth",
	Long: `
Prune blocks one by one to reduce MANIFEST growth. This command prunes blocks 
from the blockstore, state store, transaction indexer, and block indexer 
incrementally to minimize the impact on the MANIFEST file size.

This should only be run when the node is stopped.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return pruneBlockByBlock(config.RootDir, logger)
	},
}

func NewPruneBlockByBlockCmd(rootCmd *cobra.Command, hidden bool) *cobra.Command {
	flagZsh := "zsh"
	cmd := &cobra.Command{
		Use:   "prune-block-by-block",
		Short: "prune blocks one by one to reduce MANIFEST growth",
		Long: `Prune blocks one by one to reduce MANIFEST growth. This command prunes blocks 
		from the blockstore, state store, transaction indexer, and block indexer 
		incrementally to minimize the impact on the MANIFEST file size.

		This should only be run when the node is stopped.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return pruneBlockByBlock(config.RootDir, logger)
		},
		Hidden: hidden,
		Args:   cobra.NoArgs,
	}

	cmd.Flags().Bool(flagZsh, false, "Generate Zsh completion script")
	cmd.Flags().Int64Var(&fromHeight, "from", 0, "from height (inclusive)")
	cmd.Flags().Int64Var(&toHeight, "to", 0, "to height (exclusive)")
	cmd.MarkFlagRequired("from")
	cmd.MarkFlagRequired("to")

	return cmd
}

func init() {
	PruneBlockByBlockCmd.Flags().Int64Var(&fromHeight, "from", 0, "from height (inclusive)")
	PruneBlockByBlockCmd.Flags().Int64Var(&toHeight, "to", 0, "to height (exclusive)")
	PruneBlockByBlockCmd.MarkFlagRequired("from")
	PruneBlockByBlockCmd.MarkFlagRequired("to")
}

func pruneBlockByBlock(rootDir string, logger log.Logger) error {
	config := cfg.DefaultConfig()
	config.SetRoot(rootDir)

	// Load configuration
	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
	}

	// Load genesis doc to get chain ID
	// genDoc, err := types.GenesisDocFromFile(config.GenesisFile())
	// if err != nil {
	// 	return fmt.Errorf("failed to load genesis doc: %w", err)
	// }

	// // Create indexers
	// txIndexer, blockIndexer, err := block.IndexerFromConfig(config, cfg.DefaultDBProvider, genDoc.ChainID)
	// if err != nil {
	// 	return fmt.Errorf("failed to create indexers: %w", err)
	// }

	// Initialize databases
	blockStoreDB, err := cfg.DefaultDBProvider(&cfg.DBContext{
		ID:     "blockstore",
		Config: config,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize block store DB: %w", err)
	}
	defer blockStoreDB.Close()

	stateDB, err := cfg.DefaultDBProvider(&cfg.DBContext{
		ID:     "state",
		Config: config,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize state DB: %w", err)
	}
	defer stateDB.Close()

	// Initialize stores
	blockStore := store.NewBlockStore(blockStoreDB)
	stateStore := sm.NewStore(stateDB, sm.StoreOptions{
		DiscardABCIResponses: config.Storage.DiscardABCIResponses,
	})

	// Load current state
	state, err := stateStore.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	fmt.Printf("Starting block-by-block pruning from height %d to %d\n", fromHeight, toHeight)

	// Prune block by block
	for height := fromHeight; height < toHeight; height++ {
		fmt.Printf("Pruning block at height %d...\n", height)

		// Prune block store
		amountPrunedBlocks, _, prunedHeaderHeight, err := blockStore.PruneBlocks(height+1, state)
		if err != nil {
			fmt.Printf("Warning: failed to prune block store at height %d: %v\n", height, err)
		} else {
			fmt.Printf("Pruned %d blocks from block store at height %d\n", amountPrunedBlocks, height)
		}

		// Prune state store
		err = stateStore.PruneStates(height, height+1, prunedHeaderHeight)
		if err != nil {
			fmt.Printf("Warning: failed to prune state store at height %d: %v\n", height, err)
		} else {
			fmt.Printf("Pruned state store at height %d\n", height)
		}

		// // Prune transaction indexer
		// if txIndexer != nil {
		// 	err = txIndexer.PruneTransactionsFromTo(height, height+1)
		// 	if err != nil {
		// 		fmt.Printf("Warning: failed to prune tx indexer at height %d: %v\n", height, err)
		// 	} else {
		// 		fmt.Printf("Pruned tx indexer at height %d\n", height)
		// 	}
		// }

		// // Prune block indexer
		// if blockIndexer != nil {
		// 	err = blockIndexer.PruneBlocks(height, height+1)
		// 	if err != nil {
		// 		fmt.Printf("Warning: failed to prune block indexer at height %d: %v\n", height, err)
		// 	} else {
		// 		fmt.Printf("Pruned block indexer at height %d\n", height)
		// 	}
		// }

		// Force compaction after each block to reduce MANIFEST size
		if blockStoreDB != nil {
			if err := blockStoreDB.Compact(nil, nil); err != nil {
				fmt.Printf("Warning: failed to compact block store at height %d: %v\n", height, err)
			}
		}
		if stateDB != nil {
			if err := stateDB.Compact(nil, nil); err != nil {
				fmt.Printf("Warning: failed to compact state store at height %d: %v\n", height, err)
			}
		}

		fmt.Printf("Completed pruning block at height %d\n", height)
	}

	// Prune WAL files after all blocks are pruned
	walPath := filepath.Join(rootDir, "data", "cs.wal", "wal")
	if err := consensus.PruneWALFiles(walPath, toHeight); err != nil {
		fmt.Printf("Warning: failed to prune WAL files: %v\n", err)
	} else {
		fmt.Printf("Successfully pruned WAL files up to height %d\n", toHeight)
	}

	fmt.Printf("Block-by-block pruning completed from height %d to %d\n", fromHeight, toHeight)
	return nil
}
