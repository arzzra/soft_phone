package pjsua

import (
	"fmt"
	"strings"
)

// CommandTranslator translates between old and new PJSUA CLI command formats
type CommandTranslator struct {
	// Map of old commands to new hierarchical commands
	mapping map[string]string
}

// NewCommandTranslator creates a new command translator
func NewCommandTranslator() *CommandTranslator {
	return &CommandTranslator{
		mapping: map[string]string{
			// Help
			"help": "?",
			
			// Call commands
			"call":              "call new",
			"list_calls":        "call list",
			"hangup":            "call hangup",
			"answer":            "call answer",
			"hold":              "call hold",
			"reinvite":          "call reinvite",
			"update":            "call update",
			"transfer":          "call transfer",
			"transfer_replaces": "call transfer_replaces",
			"redirect":          "call redirect",
			"dtmf":              "call dtmf",
			"dump_call":         "call info",
			"call_info":         "call info",
			
			// Account commands
			"reg_dump":    "acc show",
			"acc add":     "acc add",
			"unreg":       "acc unreg",
			"rereg":       "acc reg",
			"acc_set":     "acc next",
			"acc_next":    "acc next",
			"acc_show":    "acc show",
			"acc_default": "acc default",
			
			// Audio commands
			"audio_list":       "audio dev list",
			"audio_conf":       "audio conf list",
			"audio_connect":    "audio conf connect",
			"audio_disconnect": "audio conf disconnect",
			"snd_dev":          "audio dev set",
			"codec_list":       "audio codec list",
			"codec_prio":       "audio codec prio",
			"adjust_volume":    "audio conf adjust",
			
			// IM/Presence commands
			"buddy_list":  "im buddy list",
			"sub":         "im sub",
			"unsub":       "im unsub",
			"im":          "im send",
			"typing":      "im typing",
			"online":      "im online",
			"status":      "im status",
			"buddy_add":   "im buddy add",
			"buddy_del":   "im buddy del",
			
			// Video commands
			"vid_enable":      "video enable",
			"vid_disable":     "video disable", 
			"vid_dev_list":    "video dev list",
			"vid_dev_set":     "video dev set",
			"vid_win_list":    "video win list",
			"vid_win_show":    "video win show",
			"vid_win_hide":    "video win hide",
			"vid_win_move":    "video win move",
			"vid_win_resize":  "video win resize",
			
			// Status/Config commands
			"dump_stat":       "stat dump",
			"dump_stat detail": "stat dump detail",
			"dump_settings":   "stat settings",
			"write_settings":  "stat save",
			"show_config":     "stat config",
			"stat":            "stat info",
			
			// General commands
			"sleep":     "sleep",
			"quit":      "shutdown",
			"exit":      "exit",
			"restart":   "restart",
			"echo":      "echo",
		},
	}
}

// Translate converts an old command to the new format
func (ct *CommandTranslator) Translate(oldCmd string) string {
	// First check if it's already in new format
	if ct.isNewFormat(oldCmd) {
		return oldCmd
	}
	
	// Try direct mapping
	if newCmd, ok := ct.mapping[oldCmd]; ok {
		return newCmd
	}
	
	// Try to parse command with arguments
	parts := strings.Fields(oldCmd)
	if len(parts) == 0 {
		return oldCmd
	}
	
	// Check if base command needs translation
	baseCmd := parts[0]
	if newBase, ok := ct.mapping[baseCmd]; ok {
		// Reconstruct with new base command
		parts[0] = newBase
		return strings.Join(parts, " ")
	}
	
	// Special handling for commands with parameters
	return ct.translateWithParams(oldCmd)
}

// isNewFormat checks if command is already in new hierarchical format
func (ct *CommandTranslator) isNewFormat(cmd string) bool {
	newPrefixes := []string{
		"call ", "acc ", "audio ", "im ", "video ", "stat ",
		"log ", "network ", "toggle_sdp_offer",
	}
	
	cmdLower := strings.ToLower(strings.TrimSpace(cmd))
	for _, prefix := range newPrefixes {
		if strings.HasPrefix(cmdLower, prefix) {
			return true
		}
	}
	
	// Single character commands that are valid
	if cmd == "?" || cmd == "o" {
		return true
	}
	
	return false
}

// translateWithParams handles special cases with parameters
func (ct *CommandTranslator) translateWithParams(oldCmd string) string {
	parts := strings.Fields(oldCmd)
	if len(parts) < 2 {
		return oldCmd
	}
	
	// Handle special cases
	switch parts[0] {
	case "call":
		// "call <uri>" -> "call new <uri>"
		if len(parts) >= 2 && !isSubcommand(parts[1]) {
			return fmt.Sprintf("call new %s", strings.Join(parts[1:], " "))
		}
		
	case "hangup":
		// "hangup [call_id]" -> "call hangup [call_id]"
		if len(parts) == 1 {
			return "call hangup"
		}
		return fmt.Sprintf("call hangup %s", strings.Join(parts[1:], " "))
		
	case "answer":
		// "answer [code] [call_id]" -> "call answer [code] [call_id]"
		if len(parts) == 1 {
			return "call answer"
		}
		return fmt.Sprintf("call answer %s", strings.Join(parts[1:], " "))
		
	case "transfer":
		// "transfer <uri> [call_id]" -> "call transfer <uri> [call_id]"
		return fmt.Sprintf("call transfer %s", strings.Join(parts[1:], " "))
		
	case "dtmf":
		// "dtmf <digits> [call_id]" -> "call dtmf <digits> [call_id]"
		return fmt.Sprintf("call dtmf %s", strings.Join(parts[1:], " "))
		
	case "im":
		// "im <uri> <message>" -> "im send <uri> <message>"
		if len(parts) >= 3 {
			return fmt.Sprintf("im send %s", strings.Join(parts[1:], " "))
		}
		
	case "status":
		// "status [code] [text]" -> "im status [code] [text]"
		if len(parts) >= 2 {
			return fmt.Sprintf("im status %s", strings.Join(parts[1:], " "))
		}
		return "im status"
	}
	
	return oldCmd
}

// isSubcommand checks if a string is likely a subcommand rather than parameter
func isSubcommand(s string) bool {
	subcommands := []string{
		"new", "list", "hangup", "answer", "hold", "reinvite", "update",
		"transfer", "redirect", "dtmf", "info", "show", "add", "unreg",
		"reg", "next", "default", "send", "typing", "online", "status",
		"sub", "unsub", "buddy", "dev", "conf", "codec", "enable",
		"disable", "win", "dump", "save", "settings", "config",
	}
	
	s = strings.ToLower(s)
	for _, sub := range subcommands {
		if s == sub {
			return true
		}
	}
	
	return false
}

// GetMapping returns the full command mapping for reference
func (ct *CommandTranslator) GetMapping() map[string]string {
	// Return a copy to prevent external modification
	result := make(map[string]string, len(ct.mapping))
	for k, v := range ct.mapping {
		result[k] = v
	}
	return result
}