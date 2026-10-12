package reftrace

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.trace")
	var b []byte
	b = append(b, magic...)
	b = binary.LittleEndian.AppendUint32(b, 9)
	b = binary.LittleEndian.AppendUint32(b, 9)
	for i := 0; i < 2; i++ {
		rec := make([]byte, recordSize)
		binary.LittleEndian.PutUint32(rec[0:], uint32(i))
		binary.LittleEndian.PutUint32(rec[4:], uint32(10+i))
		rec[12] = 0x02
		rec[20+7] = 0xB8              // PSG R7
		rec[36+5] = byte(0xA0 + i)    // RAM $E005
		rec[36+RAMSize+0x3800] = 0x42 // VRAM $3800
		b = append(b, rec...)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	ev := "W -1 0 4021\nP 3 57429 7\nK 5 8 1 down\n"
	if err := os.WriteFile(p+".events", []byte(ev), 0o644); err != nil {
		t.Fatal(err)
	}
	tr, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if tr.Len != 2 || tr.InitInterrupts != 9 || len(tr.Events) != 3 {
		t.Fatalf("len %d init %d events %d", tr.Len, tr.InitInterrupts, len(tr.Events))
	}
	r, err := tr.Read(1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Tick != 1 || r.Interrupts != 11 || r.Regs[0] != 2 || r.PSG[7] != 0xB8 || r.RAM[5] != 0xA1 || r.VRAM[0x3800] != 0x42 {
		t.Errorf("record: tick %d irq %d r0 %x ram %x vram %x", r.Tick, r.Interrupts, r.Regs[0], r.RAM[5], r.VRAM[0x3800])
	}
	e := tr.Events
	if e[0].Tick != -1 || e[0].Ret != 0x4021 || e[1].Addr != 0xE055 || e[1].A != 7 || !e[2].Down || e[2].Row != 8 || e[2].Mask != 1 {
		t.Errorf("events: %+v", e)
	}
	if _, err := ParseEvents(strings.NewReader("W 1 x 4021")); err == nil {
		t.Error("bad event accepted")
	}
}
