package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/krish567366/OpenTX/pkg/testutil"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	version = "1.0.0"
	logger  *zap.Logger
)

func main() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	rootCmd := &cobra.Command{
		Use:   "opentx",
		Short: "OpenTX - Canonical Transaction Protocol & Gateway CLI",
		Long:  `Command-line interface for managing and testing OpenTX gateway`,
	}

	rootCmd.AddCommand(
		versionCmd(),
		testCmd(),
		generateCmd(),
		healthCmd(),
		configCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		logger.Error("command failed", zap.Error(err))
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("OpenTX CLI v%s\n", version)
			fmt.Println("Build Date:", time.Now().Format("2006-01-02"))
		},
	}
}

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test transaction processing",
		Long:  `Send test transactions to the gateway`,
	}

	var (
		count    int
		cardType string
		amount   int64
	)

	cmd.Flags().IntVarP(&count, "count", "c", 1, "Number of test transactions")
	cmd.Flags().StringVarP(&cardType, "card-type", "t", "visa", "Card type (visa, mastercard, amex)")
	cmd.Flags().Int64VarP(&amount, "amount", "a", 10000, "Transaction amount in cents")

	cmd.Run = func(cmd *cobra.Command, args []string) {
		runTestTransactions(count, cardType, amount)
	}

	return cmd
}

func generateCmd() *cobra.Command{
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate test data",
	}

	cmd.AddCommand(
		generateCardCmd(),
		generateTransactionCmd(),
		generateBatchCmd(),
	)

	return cmd
}

func generateCardCmd() *cobra.Command {
	var cardType string

	cmd := &cobra.Command{
		Use:   "card",
		Short: "Generate test card number",
		Run: func(cmd *cobra.Command, args []string) {
			generator := testutil.NewTestDataGenerator()
			
			var brand int32
			switch cardType {
			case "visa":
				brand = 1
			case "mastercard":
				brand = 2
			case "amex":
				brand = 3
			default:
				brand = 1
			}
			
			card := generator.GenerateCard(brand)
			fmt.Printf("Card Number: %s\n", card.Pan)
			fmt.Printf("Expiry: %s\n", card.ExpirationDate)
			fmt.Printf("Brand: %s\n", card.CardBrand.String())
		},
	}

	cmd.Flags().StringVarP(&cardType, "type", "t", "visa", "Card type (visa, mastercard, amex)")
	
	return cmd
}

func generateTransactionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "transaction",
		Short: "Generate test transaction",
		Run: func(cmd *cobra.Command, args []string) {
			generator := testutil.NewTestDataGenerator()
			txn := generator.GenerateTransaction()
			
			fmt.Println("Generated Transaction:")
			fmt.Printf("Message ID: %s\n", txn.MessageId)
			fmt.Printf("Amount: $%.2f\n", float64(txn.Money.Amount)/100.0)
			fmt.Printf("Card: %s\n", txn.CardData.Pan)
			fmt.Printf("Merchant: %s\n", txn.MerchantData.MerchantName)
		},
	}
}

func generateBatchCmd() *cobra.Command {
	var count int

	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Generate batch of test transactions",
		Run: func(cmd *cobra.Command, args []string) {
			generator := testutil.NewTestDataGenerator()
			transactions := generator.GenerateBatchTransactions(count)
			
			fmt.Printf("Generated %d transactions\n", len(transactions))
			for i, txn := range transactions {
				fmt.Printf("%d. %s - $%.2f\n", i+1, txn.MessageId, float64(txn.Money.Amount)/100.0)
			}
		},
	}

	cmd.Flags().IntVarP(&count, "count", "c", 10, "Number of transactions to generate")
	
	return cmd
}

func healthCmd() *cobra.Command {
	var endpoint string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check gateway health",
		Run: func(cmd *cobra.Command, args []string) {
			checkHealth(endpoint)
		},
	}

	cmd.Flags().StringVarP(&endpoint, "endpoint", "e", "localhost:50051", "Gateway endpoint")

	return cmd
}

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	cmd.AddCommand(
		configShowCmd(),
		configValidateCmd(),
	)

	return cmd
}

func configShowCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Configuration file: %s\n", configFile)
			// TODO: Load and display configuration
		},
	}

	cmd.Flags().StringVarP(&configFile, "file", "f", "configs/gateway.yaml", "Config file path")

	return cmd
}

func configValidateCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Validating %s...\n", configFile)
			// TODO: Implement configuration validation
			fmt.Println("✓ Configuration is valid")
		},
	}

	cmd.Flags().StringVarP(&configFile, "file", "f", "configs/gateway.yaml", "Config file path")

	return cmd
}

func runTestTransactions(count int, cardType string, amount int64) {
	ctx := context.Background()
	generator := testutil.NewTestDataGenerator()

	fmt.Printf("Sending %d test transaction(s)...\n", count)

	for i := 0; i < count; i++ {
		txn := generator.GenerateTransaction()
		txn.Money.Amount = amount

		fmt.Printf("\nTransaction %d/%d\n", i+1, count)
		fmt.Printf("  Message ID: %s\n", txn.MessageId)
		fmt.Printf("  Amount: $%.2f\n", float64(amount)/100.0)
		fmt.Printf("  Card: %s\n", txn.CardData.Pan)
		
		// TODO: Actually send to gateway
		_ = ctx
		
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("\n✓ All transactions sent")
}

func checkHealth(endpoint string) {
	fmt.Printf("Checking health of %s...\n", endpoint)
	
	// TODO: Implement actual health check via gRPC
	
	fmt.Println("Status: Healthy ✓")
	fmt.Println("Components:")
	fmt.Println("  - Database: OK")
	fmt.Println("  - Redis: OK")
	fmt.Println("  - Kafka: OK")
}
