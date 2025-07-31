package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/state/indexer/block"
	"github.com/cometbft/cometbft/types"
)

var fromHeight, toHeight int64
var pruneBlockIndex bool

func init() {
	PruneTxIndexCmd.Flags().Int64Var(&fromHeight, "from", 0, "from height (inclusive)")
	PruneTxIndexCmd.Flags().Int64Var(&toHeight, "to", 0, "to height (exclusive)")
	PruneTxIndexCmd.Flags().BoolVar(&pruneBlockIndex, "block-index", false, "prune block index instead of tx index")
	PruneTxIndexCmd.MarkFlagRequired("from")
	PruneTxIndexCmd.MarkFlagRequired("to")
}

var PruneTxIndexCmd = &cobra.Command{
	Use:   "prune-txindex",
	Short: "prune transaction or block index for a range of heights",
	Long: `
Prune transaction or block index for heights from 'from' (inclusive) to 'to' (exclusive).
This will remove all transaction or block data for the specified height range from the indexer.

Example:
  cometbft prune-txindex --from 1000 --to 2000
  cometbft prune-txindex --from 1000 --to 2000 --block-index
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := cfg.DefaultConfig()
		config.SetRoot(config.RootDir)

		// Load configuration
		if err := viper.ReadInConfig(); err == nil {
			fmt.Printf("Using config file: %s\n", viper.ConfigFileUsed())
		}

		// Load genesis doc to get chain ID
		genDoc, err := types.GenesisDocFromFile(config.GenesisFile())
		if err != nil {
			return fmt.Errorf("failed to load genesis doc: %w", err)
		}

		// Create indexers
		txIndexer, blockIndexer, err := block.IndexerFromConfig(config, cfg.DefaultDBProvider, genDoc.ChainID)
		if err != nil {
			return fmt.Errorf("failed to create indexers: %w", err)
		}

		// Validate heights
		if fromHeight <= 0 || toHeight <= 0 {
			return fmt.Errorf("from height %v and to height %v must be greater than 0", fromHeight, toHeight)
		}
		if fromHeight >= toHeight {
			return fmt.Errorf("from height %v must be lower than to height %v", fromHeight, toHeight)
		}

		if pruneBlockIndex {
			fmt.Printf("Pruning block index from height %d to %d...\n", fromHeight, toHeight)

			// Prune blocks
			err = blockIndexer.PruneBlocks(fromHeight, toHeight)
			if err != nil {
				return fmt.Errorf("failed to prune block index: %w", err)
			}

			fmt.Printf("Successfully pruned block index from height %d to %d\n", fromHeight, toHeight)
		} else {
			fmt.Printf("Pruning transaction index from height %d to %d...\n", fromHeight, toHeight)

			// Prune transactions (empty hash list for manual pruning)
			err = txIndexer.PruneTransactionsFromTo(fromHeight, toHeight)
			if err != nil {
				return fmt.Errorf("failed to prune transaction index: %w", err)
			}

			fmt.Printf("Successfully pruned transaction index from height %d to %d\n", fromHeight, toHeight)
		}

		return nil
	},
}
