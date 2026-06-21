package protocol

import (
	"encoding/binary"
	"io"
)

const (
	ChannelTypeRead    uint8 = 0x01
	ChannelTypeWrite   uint8 = 0x02
	ChannelTypeControl uint8 = 0x03

	MsgData      uint8 = 0x01
	MsgHeartbeat uint8 = 0x02
	MsgStats     uint8 = 0x03
	MsgHandshake uint8 = 0x04
)

type Header struct {
	ChannelID   uint16
	ChannelType uint8
	MsgType     uint8
	SeqNum      uint32
	Length      uint32
}

const HeaderSize = 12

type Packet struct {
	Header
	Data []byte
}

func ReadPacket(r io.Reader) (*Packet, error) {
	hdr := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}

	p := &Packet{
		Header: Header{
			ChannelID:   binary.BigEndian.Uint16(hdr[0:2]),
			ChannelType: hdr[2],
			MsgType:     hdr[3],
			SeqNum:      binary.BigEndian.Uint32(hdr[4:8]),
			Length:      binary.BigEndian.Uint32(hdr[8:12]),
		},
	}

	if p.Length > 0 {
		p.Data = make([]byte, p.Length)
		if _, err := io.ReadFull(r, p.Data); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func WritePacket(w io.Writer, p *Packet) error {
	hdr := make([]byte, HeaderSize)
	binary.BigEndian.PutUint16(hdr[0:2], p.ChannelID)
	hdr[2] = p.ChannelType
	hdr[3] = p.MsgType
	binary.BigEndian.PutUint32(hdr[4:8], p.SeqNum)
	binary.BigEndian.PutUint32(hdr[8:12], uint32(len(p.Data)))

	if _, err := w.Write(hdr); err != nil {
		return err
	}

	if len(p.Data) > 0 {
		if _, err := w.Write(p.Data); err != nil {
			return err
		}
	}

	return nil
}

type Handshake struct {
	Version     uint32
	ClientID    [16]byte
	ReadRatio   float64
	WriteRatio  float64
	MinChannels uint32
	MaxChannels uint32
}

func ReadHandshake(r io.Reader) (*Handshake, error) {
	data := make([]byte, 44)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	h := &Handshake{
		Version:     binary.BigEndian.Uint32(data[0:4]),
		ReadRatio:   float64(binary.BigEndian.Uint32(data[20:24])) / 1000.0,
		WriteRatio:  float64(binary.BigEndian.Uint32(data[24:28])) / 1000.0,
		MinChannels: binary.BigEndian.Uint32(data[28:32]),
		MaxChannels: binary.BigEndian.Uint32(data[32:36]),
	}
	copy(h.ClientID[:], data[4:20])

	return h, nil
}

func WriteHandshake(w io.Writer, h *Handshake) error {
	data := make([]byte, 44)
	binary.BigEndian.PutUint32(data[0:4], h.Version)
	copy(data[4:20], h.ClientID[:])
	binary.BigEndian.PutUint32(data[20:24], uint32(h.ReadRatio*1000))
	binary.BigEndian.PutUint32(data[24:28], uint32(h.WriteRatio*1000))
	binary.BigEndian.PutUint32(data[28:32], h.MinChannels)
	binary.BigEndian.PutUint32(data[32:36], h.MaxChannels)

	_, err := w.Write(data)
	return err
}
