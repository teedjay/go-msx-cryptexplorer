// Package msxnames names MSX1 BIOS entry points and system variables, for
// the disassembler's call targets and memory operands respectively.
package msxnames

import "fmt"

// BIOS maps MSX1 main-ROM entry points to their documented names.
var BIOS = map[uint16]string{
	0x0000: "CHKRAM", 0x0008: "SYNCHR", 0x000C: "RDSLT", 0x0010: "CHRGTR",
	0x0014: "WRSLT", 0x0018: "OUTDO", 0x001C: "CALSLT", 0x0020: "DCOMPR",
	0x0024: "ENASLT", 0x0028: "GETYPR", 0x0030: "CALLF", 0x0038: "KEYINT",
	0x003B: "INITIO", 0x003E: "INIFNK", 0x0041: "DISSCR", 0x0044: "ENASCR",
	0x0047: "WRTVDP", 0x004A: "RDVRM", 0x004D: "WRTVRM", 0x0050: "SETRD",
	0x0053: "SETWRT", 0x0056: "FILVRM", 0x0059: "LDIRMV", 0x005C: "LDIRVM",
	0x005F: "CHGMOD", 0x0062: "CHGCLR", 0x0066: "NMI", 0x0069: "CLRSPR",
	0x006C: "INITXT", 0x006F: "INIT32", 0x0072: "INIGRP", 0x0075: "INIMLT",
	0x0078: "SETTXT", 0x007B: "SETT32", 0x007E: "SETGRP", 0x0081: "SETMLT",
	0x0084: "CALPAT", 0x0087: "CALATR", 0x008A: "GSPSIZ", 0x008D: "GRPPRT",
	0x0090: "GICINI", 0x0093: "WRTPSG", 0x0096: "RDPSG", 0x0099: "STRTMS",
	0x009C: "CHSNS", 0x009F: "CHGET", 0x00A2: "CHPUT", 0x00A5: "LPTOUT",
	0x00A8: "LPTSTT", 0x00AB: "CNVCHR", 0x00AE: "PINLIN", 0x00B1: "INLIN",
	0x00B4: "QINLIN", 0x00B7: "BREAKX", 0x00BA: "ISCNTC", 0x00BD: "CKCNTC",
	0x00C0: "BEEP", 0x00C3: "CLS", 0x00C6: "POSIT", 0x00C9: "FNKSB",
	0x00CC: "ERAFNK", 0x00CF: "DSPFNK", 0x00D2: "TOTEXT", 0x00D5: "GTSTCK",
	0x00D8: "GTTRIG", 0x00DB: "GTPAD", 0x00DE: "GTPDL", 0x00E1: "TAPION",
	0x00E4: "TAPIN", 0x00E7: "TAPIOF", 0x00EA: "TAPOON", 0x00ED: "TAPOUT",
	0x00F0: "TAPOOF", 0x00F3: "STMOTR", 0x00F6: "LFTQ", 0x00F9: "PUTQ",
	0x00FC: "RIGHTC", 0x00FF: "LEFTC", 0x0102: "UPC", 0x0105: "TUPC",
	0x0108: "DOWNC", 0x010B: "TDOWNC", 0x010E: "SCALXY", 0x0111: "MAPXYC",
	0x0114: "FETCHC", 0x0117: "STOREC", 0x011A: "SETATR", 0x011D: "READC",
	0x0120: "SETC", 0x0123: "NSETCX", 0x0126: "GTASPC", 0x0129: "PNTINI",
	0x012C: "SCANR", 0x012F: "SCANL", 0x0132: "CHGCAP", 0x0135: "CHGSND",
	0x0138: "RSLREG", 0x013B: "WSLREG", 0x013E: "RDVDP", 0x0141: "SNSMAT",
	0x0144: "PHYDIO", 0x0147: "FORMAT", 0x014A: "ISFLIO", 0x014D: "OUTDLP",
	0x0150: "GETVCP", 0x0153: "GETVC2", 0x0156: "KILBUF", 0x0159: "CALBAS",
}

type sysVar struct {
	addr uint16
	size uint16
	name string
}

// sysVars lists the system variables and hooks a cartridge game is likely
// to touch, with their sizes so that interior bytes get NAME+n.
var sysVars = []sysVar{
	{0x0006, 1, "VDPDR"}, {0x0007, 1, "VDPDW"},
	{0xF3AE, 1, "LINL40"}, {0xF3AF, 1, "LINL32"}, {0xF3B0, 1, "LINLEN"},
	{0xF3DB, 1, "CLIKSW"}, {0xF3DC, 1, "CSRY"}, {0xF3DD, 1, "CSRX"},
	{0xF3DF, 1, "RG0SAV"}, {0xF3E0, 1, "RG1SAV"}, {0xF3E1, 1, "RG2SAV"},
	{0xF3E2, 1, "RG3SAV"}, {0xF3E3, 1, "RG4SAV"}, {0xF3E4, 1, "RG5SAV"},
	{0xF3E5, 1, "RG6SAV"}, {0xF3E6, 1, "RG7SAV"}, {0xF3E7, 1, "STATFL"},
	{0xF3E8, 1, "TRGFLG"}, {0xF3E9, 1, "FORCLR"}, {0xF3EA, 1, "BAKCLR"},
	{0xF3EB, 1, "BDRCLR"}, {0xF3EC, 1, "MAXUPD"}, {0xF3FC, 5, "CS120"},
	{0xFBB0, 1, "ENSTOP"}, {0xFBDA, 11, "OLDKEY"}, {0xFBE5, 11, "NEWKEY"},
	{0xFC48, 2, "BOTTOM"}, {0xFC4A, 2, "HIMEM"}, {0xFC9E, 2, "JIFFY"},
	{0xFCA9, 1, "CSRSW"}, {0xFCAB, 1, "CAPST"}, {0xFCAC, 1, "KANAST"},
	{0xFCAF, 1, "SCRMOD"}, {0xFCC1, 4, "EXPTBL"}, {0xFCC5, 4, "SLTTBL"},
	{0xFD9A, 5, "H.KEYI"}, {0xFD9F, 5, "H.TIMI"},
	{0xFFFF, 1, "SSLREG"},
}

// SysVar names a memory operand inside a known system variable, as NAME or
// NAME+n. It returns "" for anything else.
func SysVar(a uint16) string {
	for _, v := range sysVars {
		if a >= v.addr && a < v.addr+v.size {
			if a == v.addr {
				return v.name
			}
			return fmt.Sprintf("%s+%d", v.name, a-v.addr)
		}
	}
	return ""
}
