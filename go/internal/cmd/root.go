package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	Version   string
	BuildTime string
	cfgFile   string
)

var (
	loginMode bool
)

var rootCmd = &cobra.Command{
	Use:   "antigravity",
	Short: "Antigravity API to OpenAI format proxy server",
	Long: `Antigravity API Proxy is a service that converts Antigravity API 
to OpenAI-compatible format, with comprehensive management features.`,
	RunE: defaultRun, // 默认执行serve或login
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// 全局标志
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().String("data-dir", "./data", "data directory")
	rootCmd.PersistentFlags().String("log-dir", "./logs", "log directory")

	// OAuth登录标志
	rootCmd.Flags().BoolVar(&loginMode, "login", false, "trigger OAuth login and exit")

	// 服务器标志（直接在root命令使用）
	rootCmd.Flags().String("host", "0.0.0.0", "server host")
	rootCmd.Flags().Int("port", 8045, "server port")
	rootCmd.Flags().String("mode", "release", "server mode (debug/release/test)")

	// 绑定到viper
	viper.BindPFlag("storage.data_dir", rootCmd.PersistentFlags().Lookup("data-dir"))
	viper.BindPFlag("storage.logs_dir", rootCmd.PersistentFlags().Lookup("log-dir"))
	viper.BindPFlag("server.host", rootCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.port", rootCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.mode", rootCmd.Flags().Lookup("mode"))
}

// defaultRun 默认运行逻辑：如果指定--login则执行OAuth，否则启动服务器
func defaultRun(cmd *cobra.Command, args []string) error {
	if loginMode {
		return runLogin(cmd, args)
	}
	return runServe(cmd, args)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./data")
		viper.AddConfigPath("$HOME/.antigravity")
	}

	viper.AutomaticEnv()

	// 尝试读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		// 配置文件不存在，触发创建
		// 注意：LoadOrCreate会在serve/login命令中被调用
		// 这里只是设置viper的默认值，实际的文件创建在命令执行时进行
		if cfgFile == "" {
			// 只在使用默认配置文件路径时设置配置文件路径
			viper.SetConfigFile("./config.yaml")
		}
	} else {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
