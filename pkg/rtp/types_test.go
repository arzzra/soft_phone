package rtp

import (
	"testing"
)

func TestDirection_String(t *testing.T) {
	tests := []struct {
		name      string
		direction Direction
		want      string
	}{
		{
			name:      "SendRecv",
			direction: DirectionSendRecv,
			want:      "sendrecv",
		},
		{
			name:      "SendOnly",
			direction: DirectionSendOnly,
			want:      "sendonly",
		},
		{
			name:      "RecvOnly",
			direction: DirectionRecvOnly,
			want:      "recvonly",
		},
		{
			name:      "Inactive",
			direction: DirectionInactive,
			want:      "inactive",
		},
		{
			name:      "Unknown",
			direction: Direction(999),
			want:      "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.direction.String()
			if got != tt.want {
				t.Errorf("Direction.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirection_CanSend(t *testing.T) {
	tests := []struct {
		name      string
		direction Direction
		want      bool
	}{
		{
			name:      "SendRecv can send",
			direction: DirectionSendRecv,
			want:      true,
		},
		{
			name:      "SendOnly can send",
			direction: DirectionSendOnly,
			want:      true,
		},
		{
			name:      "RecvOnly cannot send",
			direction: DirectionRecvOnly,
			want:      false,
		},
		{
			name:      "Inactive cannot send",
			direction: DirectionInactive,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.direction.CanSend()
			if got != tt.want {
				t.Errorf("Direction.CanSend() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirection_CanReceive(t *testing.T) {
	tests := []struct {
		name      string
		direction Direction
		want      bool
	}{
		{
			name:      "SendRecv can receive",
			direction: DirectionSendRecv,
			want:      true,
		},
		{
			name:      "SendOnly cannot receive",
			direction: DirectionSendOnly,
			want:      false,
		},
		{
			name:      "RecvOnly can receive",
			direction: DirectionRecvOnly,
			want:      true,
		},
		{
			name:      "Inactive cannot receive",
			direction: DirectionInactive,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.direction.CanReceive()
			if got != tt.want {
				t.Errorf("Direction.CanReceive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirection_Constants(t *testing.T) {
	// Проверяем значения констант
	if DirectionSendRecv != 0 {
		t.Errorf("DirectionSendRecv = %d, want 0", DirectionSendRecv)
	}
	if DirectionSendOnly != 1 {
		t.Errorf("DirectionSendOnly = %d, want 1", DirectionSendOnly)
	}
	if DirectionRecvOnly != 2 {
		t.Errorf("DirectionRecvOnly = %d, want 2", DirectionRecvOnly)
	}
	if DirectionInactive != 3 {
		t.Errorf("DirectionInactive = %d, want 3", DirectionInactive)
	}
}