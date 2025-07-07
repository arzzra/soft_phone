package pjsua

import (
	"fmt"
	"strings"
)

// Command represents a PJSUA CLI command
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Category    CommandCategory
	Handler     CommandHandler
}

// CommandCategory represents the category of a command
type CommandCategory string

const (
	CategoryCall      CommandCategory = "CALL"
	CategoryIM        CommandCategory = "IM"
	CategoryPresence  CommandCategory = "PRESENCE"
	CategoryAccount   CommandCategory = "ACCOUNT"
	CategoryMedia     CommandCategory = "MEDIA"
	CategoryConfig    CommandCategory = "CONFIG"
	CategoryVideo     CommandCategory = "VIDEO"
	CategoryGeneral   CommandCategory = "GENERAL"
)

// CommandHandler is a function that handles command execution
type CommandHandler func(args []string) (string, error)

// Commands contains all PJSUA CLI commands
var Commands = map[string]*Command{
	// Call Commands
	"call": {
		Name:        "call",
		Aliases:     []string{"m"},
		Description: "Make a new call",
		Usage:       "call <URI>",
		Category:    CategoryCall,
	},
	"hangup": {
		Name:        "hangup",
		Aliases:     []string{"h"},
		Description: "Hangup current call",
		Usage:       "hangup [call_id]",
		Category:    CategoryCall,
	},
	"answer": {
		Name:        "answer",
		Aliases:     []string{"a"},
		Description: "Answer incoming call",
		Usage:       "answer [code] [call_id]",
		Category:    CategoryCall,
	},
	"hold": {
		Name:        "hold",
		Aliases:     []string{"H"},
		Description: "Hold current call",
		Usage:       "hold [call_id]",
		Category:    CategoryCall,
	},
	"reinvite": {
		Name:        "reinvite",
		Aliases:     []string{"v"},
		Description: "Send re-INVITE",
		Usage:       "reinvite [call_id]",
		Category:    CategoryCall,
	},
	"update": {
		Name:        "update",
		Aliases:     []string{"U"},
		Description: "Send UPDATE",
		Usage:       "update [call_id]",
		Category:    CategoryCall,
	},
	"transfer": {
		Name:        "transfer",
		Aliases:     []string{"xfer", "x"},
		Description: "Transfer call",
		Usage:       "transfer <URI> [call_id]",
		Category:    CategoryCall,
	},
	"transfer_replaces": {
		Name:        "transfer_replaces",
		Aliases:     []string{"X"},
		Description: "Transfer call with replaces",
		Usage:       "transfer_replaces <call_id_to_replace> [call_id]",
		Category:    CategoryCall,
	},
	"redirect": {
		Name:        "redirect",
		Aliases:     []string{"R"},
		Description: "Redirect incoming call",
		Usage:       "redirect <URI> [code] [call_id]",
		Category:    CategoryCall,
	},
	"dtmf": {
		Name:        "dtmf",
		Aliases:     []string{"#"},
		Description: "Send DTMF digits",
		Usage:       "dtmf <digits> [call_id]",
		Category:    CategoryCall,
	},
	"info": {
		Name:        "info",
		Aliases:     []string{},
		Description: "Send INFO request",
		Usage:       "info [call_id]",
		Category:    CategoryCall,
	},
	"dump_call": {
		Name:        "dump_call",
		Aliases:     []string{"dc"},
		Description: "Dump call statistics",
		Usage:       "dump_call [call_id]",
		Category:    CategoryCall,
	},
	"list_calls": {
		Name:        "list_calls",
		Aliases:     []string{"cl"},
		Description: "List all calls",
		Usage:       "list_calls",
		Category:    CategoryCall,
	},

	// Instant Messaging Commands
	"im": {
		Name:        "im",
		Aliases:     []string{"i"},
		Description: "Send instant message",
		Usage:       "im <URI> <message>",
		Category:    CategoryIM,
	},
	"typing": {
		Name:        "typing",
		Aliases:     []string{},
		Description: "Send typing indication",
		Usage:       "typing <URI> [is_typing]",
		Category:    CategoryIM,
	},

	// Presence Commands
	"sub": {
		Name:        "sub",
		Aliases:     []string{"+b"},
		Description: "Subscribe to buddy presence",
		Usage:       "sub <URI>",
		Category:    CategoryPresence,
	},
	"unsub": {
		Name:        "unsub",
		Aliases:     []string{"-b"},
		Description: "Unsubscribe buddy presence",
		Usage:       "unsub <buddy_id>",
		Category:    CategoryPresence,
	},
	"toggle_state": {
		Name:        "toggle_state",
		Aliases:     []string{"t"},
		Description: "Toggle buddy subscription",
		Usage:       "toggle_state <buddy_id>",
		Category:    CategoryPresence,
	},
	"online": {
		Name:        "online",
		Aliases:     []string{"o"},
		Description: "Set online status",
		Usage:       "online",
		Category:    CategoryPresence,
	},
	"status": {
		Name:        "status",
		Aliases:     []string{},
		Description: "Set presence status",
		Usage:       "status [code] [text]",
		Category:    CategoryPresence,
	},
	"buddy_list": {
		Name:        "buddy_list",
		Aliases:     []string{"bl", "l"},
		Description: "List all buddies",
		Usage:       "buddy_list",
		Category:    CategoryPresence,
	},

	// Account Commands
	"acc": {
		Name:        "acc",
		Aliases:     []string{"+a"},
		Description: "Add account",
		Usage:       "acc add <sip:user@domain> [registrar]",
		Category:    CategoryAccount,
	},
	"unreg": {
		Name:        "unreg",
		Aliases:     []string{"-a"},
		Description: "Unregister account",
		Usage:       "unreg [acc_id]",
		Category:    CategoryAccount,
	},
	"rereg": {
		Name:        "rereg",
		Aliases:     []string{"rr"},
		Description: "Re-register account",
		Usage:       "rereg [acc_id]",
		Category:    CategoryAccount,
	},
	"reg_dump": {
		Name:        "reg_dump",
		Aliases:     []string{"rd"},
		Description: "Dump registration info",
		Usage:       "reg_dump",
		Category:    CategoryAccount,
	},
	"acc_show": {
		Name:        "acc_show",
		Aliases:     []string{},
		Description: "Show account details",
		Usage:       "acc_show [acc_id]",
		Category:    CategoryAccount,
	},
	"acc_set": {
		Name:        "acc_set",
		Aliases:     []string{"<"},
		Description: "Select default account",
		Usage:       "acc_set <acc_id>",
		Category:    CategoryAccount,
	},

	// Media Commands
	"audio_list": {
		Name:        "audio_list",
		Aliases:     []string{},
		Description: "List audio devices",
		Usage:       "audio_list",
		Category:    CategoryMedia,
	},
	"audio_conf": {
		Name:        "audio_conf",
		Aliases:     []string{"cc"},
		Description: "List conference ports",
		Usage:       "audio_conf",
		Category:    CategoryMedia,
	},
	"audio_connect": {
		Name:        "audio_connect",
		Aliases:     []string{"cd"},
		Description: "Connect conference ports",
		Usage:       "audio_connect <src_port> <dst_port>",
		Category:    CategoryMedia,
	},
	"audio_disconnect": {
		Name:        "audio_disconnect",
		Aliases:     []string{"cd"},
		Description: "Disconnect conference ports",
		Usage:       "audio_disconnect <src_port> <dst_port>",
		Category:    CategoryMedia,
	},
	"adjust_volume": {
		Name:        "adjust_volume",
		Aliases:     []string{"V"},
		Description: "Adjust volume",
		Usage:       "adjust_volume <port> <level>",
		Category:    CategoryMedia,
	},
	"codec_list": {
		Name:        "codec_list",
		Aliases:     []string{},
		Description: "List codecs",
		Usage:       "codec_list",
		Category:    CategoryMedia,
	},
	"codec_prio": {
		Name:        "codec_prio",
		Aliases:     []string{"Cp"},
		Description: "Set codec priority",
		Usage:       "codec_prio <codec_id> <priority>",
		Category:    CategoryMedia,
	},
	"media_stats": {
		Name:        "media_stats",
		Aliases:     []string{},
		Description: "Show media statistics",
		Usage:       "media_stats [call_id]",
		Category:    CategoryMedia,
	},
	"snd_dev": {
		Name:        "snd_dev",
		Aliases:     []string{},
		Description: "Set sound device",
		Usage:       "snd_dev <capture_dev> <playback_dev>",
		Category:    CategoryMedia,
	},

	// Video Commands
	"vid_enable": {
		Name:        "vid_enable",
		Aliases:     []string{},
		Description: "Enable video",
		Usage:       "vid_enable [call_id]",
		Category:    CategoryVideo,
	},
	"vid_disable": {
		Name:        "vid_disable",
		Aliases:     []string{},
		Description: "Disable video",
		Usage:       "vid_disable [call_id]",
		Category:    CategoryVideo,
	},
	"vid_dev_list": {
		Name:        "vid_dev_list",
		Aliases:     []string{"vl"},
		Description: "List video devices",
		Usage:       "vid_dev_list",
		Category:    CategoryVideo,
	},
	"vid_dev_set": {
		Name:        "vid_dev_set",
		Aliases:     []string{},
		Description: "Set video device",
		Usage:       "vid_dev_set <dev_id>",
		Category:    CategoryVideo,
	},
	"vid_codec_list": {
		Name:        "vid_codec_list",
		Aliases:     []string{},
		Description: "List video codecs",
		Usage:       "vid_codec_list",
		Category:    CategoryVideo,
	},
	"vid_codec_prio": {
		Name:        "vid_codec_prio",
		Aliases:     []string{},
		Description: "Set video codec priority",
		Usage:       "vid_codec_prio <codec> <priority>",
		Category:    CategoryVideo,
	},
	"vid_win_list": {
		Name:        "vid_win_list",
		Aliases:     []string{},
		Description: "List video windows",
		Usage:       "vid_win_list",
		Category:    CategoryVideo,
	},
	"vid_win_show": {
		Name:        "vid_win_show",
		Aliases:     []string{},
		Description: "Show video window",
		Usage:       "vid_win_show <win_id>",
		Category:    CategoryVideo,
	},
	"vid_win_hide": {
		Name:        "vid_win_hide",
		Aliases:     []string{},
		Description: "Hide video window",
		Usage:       "vid_win_hide <win_id>",
		Category:    CategoryVideo,
	},
	"vid_win_move": {
		Name:        "vid_win_move",
		Aliases:     []string{},
		Description: "Move video window",
		Usage:       "vid_win_move <win_id> <x> <y>",
		Category:    CategoryVideo,
	},
	"vid_win_resize": {
		Name:        "vid_win_resize",
		Aliases:     []string{},
		Description: "Resize video window",
		Usage:       "vid_win_resize <win_id> <width> <height>",
		Category:    CategoryVideo,
	},

	// General Commands
	"help": {
		Name:        "help",
		Aliases:     []string{"?"},
		Description: "Show help",
		Usage:       "help [command]",
		Category:    CategoryGeneral,
	},
	"quit": {
		Name:        "quit",
		Aliases:     []string{"q"},
		Description: "Quit application",
		Usage:       "quit",
		Category:    CategoryGeneral,
	},
	"restart": {
		Name:        "restart",
		Aliases:     []string{},
		Description: "Restart application",
		Usage:       "restart",
		Category:    CategoryGeneral,
	},
	"echo": {
		Name:        "echo",
		Aliases:     []string{},
		Description: "Echo text",
		Usage:       "echo <text>",
		Category:    CategoryGeneral,
	},
	"sleep": {
		Name:        "sleep",
		Aliases:     []string{},
		Description: "Sleep for specified duration",
		Usage:       "sleep <seconds>",
		Category:    CategoryGeneral,
	},
	"dump_stat": {
		Name:        "dump_stat",
		Aliases:     []string{"ds"},
		Description: "Dump statistics",
		Usage:       "dump_stat [detail]",
		Category:    CategoryGeneral,
	},
	"dump_settings": {
		Name:        "dump_settings",
		Aliases:     []string{"dq"},
		Description: "Dump settings",
		Usage:       "dump_settings",
		Category:    CategoryGeneral,
	},
	"write_settings": {
		Name:        "write_settings",
		Aliases:     []string{"f"},
		Description: "Write settings to file",
		Usage:       "write_settings <filename>",
		Category:    CategoryGeneral,
	},
}

// GetCommand returns a command by name or alias
func GetCommand(name string) (*Command, bool) {
	// Check direct name match
	if cmd, ok := Commands[name]; ok {
		return cmd, true
	}
	
	// Check aliases
	for _, cmd := range Commands {
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd, true
			}
		}
	}
	
	return nil, false
}

// FormatCommand formats a command with its arguments
func FormatCommand(cmd string, args ...interface{}) string {
	if len(args) == 0 {
		return cmd
	}
	
	strArgs := make([]string, len(args))
	for i, arg := range args {
		strArgs[i] = fmt.Sprintf("%v", arg)
	}
	
	return fmt.Sprintf("%s %s", cmd, strings.Join(strArgs, " "))
}

// ParseCommandLine parses a command line into command and arguments
func ParseCommandLine(line string) (string, []string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", nil
	}
	
	return parts[0], parts[1:]
}

// ValidateCommand checks if a command is valid
func ValidateCommand(cmd string) error {
	if _, ok := GetCommand(cmd); !ok {
		return fmt.Errorf("unknown command: %s", cmd)
	}
	return nil
}