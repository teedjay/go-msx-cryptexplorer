# Reference recorder: runs the original in openMSX (C-BIOS_MSX1_JP) purely
# as an offline oracle and records its state at the start of every game
# tick. Nothing here is part of the port.
#
# Parameters, from the environment (openMSX runs -script before -command):
#   RD_OUT    trace file to write (keep it outside the repo)
#   RD_TICKS  number of ticks to record (default 600)
#   RD_KEYS   optional input schedule, lines "tick:row:mask:down|up"
#
# Run with scripts/refdump.sh, which adds a watchdog (macOS has no timeout).
#
# Trace format (all integers little-endian):
#   header  "CXTRACE1" (8 bytes)
#           u32 interrupts (H.KEYI entries) before INIT's final EI at $40C8
#           u32 per-interrupt work ($49C0) entries before that EI
#   record per tick, at the tick-body start $403E:
#           u32 tick, u32 interrupts so far, u32 $49C0 entries so far
#           8 bytes VDP R0-R7, 16 bytes PSG R0-R15,
#           4096 bytes RAM $E000-$EFFF, 16384 bytes VRAM
# Events go to $::rd_out.events, one per line. <tick> is the tick in
# progress (the last one recorded; -1 between INIT's EI and tick 0), and
# <vw> counts VRAM data writes (OUT ($98)) since that tick's record:
#   I <tick> <vw> <pc> <busy> <ss>    interrupt taken ($0038) with <pc> the
#                                     interrupted instruction and <busy> the
#                                     busy flag ($E005 bit 0): 0 means this
#                                     interrupt starts a tick (usually at
#                                     the idle JR $ at $40C9, but also in
#                                     the handler's exit path, e.g. inside
#                                     the BIOS RDVDP called at $4048).
#                                     <ss> counts sound events since the
#                                     tick's record: sound starts ($490C,
#                                     $4901) and the game's direct reads of
#                                     the sound blocks ($41A4, $420D,
#                                     $57E4, $5BD4, $79DC, $8FA8). With
#                                     <vw> it places the interrupt among
#                                     the writes and the sound events. Before INIT's EI,
#                                     <tick> is -2 and <vw>/<ss> count
#                                     from INIT's entry ($407E)
#   W <tick> <vw> <ret>               $49C0 entered; <ret> is its return
#                                     address ($4035 handler entry, $4050
#                                     handler exit path, else elsewhere)
#   R <tick> <A>                      LD A,R at $93AE gave A
#   K <tick+1> <row> <mask> down|up   input applied before recording tick+1
#
# Every global is prefixed rd_ (globals named ::keys or ::t broke a script
# silently in the third project).

set ::rd_irq 0
set ::rd_work 0
set ::rd_tick 0
set ::rd_vw 0
set ::rd_started 0
set ::rd_ininit 0
set ::rd_ss 0
set ::rd_sched [dict create]

proc rd_env {name def} {
	if {[info exists ::env($name)]} { return $::env($name) }
	return $def
}
set ::rd_out [rd_env RD_OUT ""]
set ::rd_ticks [rd_env RD_TICKS 600]
set ::rd_keys [rd_env RD_KEYS ""]
if {$::rd_out eq ""} {
	puts stderr "refdump: set RD_OUT"
	exit 1
}

if {$::rd_keys ne ""} {
	set rd_f [open $::rd_keys r]
	foreach rd_line [split [read $rd_f] "\n"] {
		set rd_line [string trim $rd_line]
		if {$rd_line eq "" || [string index $rd_line 0] eq "#"} continue
		lassign [split $rd_line ":"] rd_t rd_row rd_mask rd_dir
		dict lappend ::rd_sched $rd_t [list $rd_row $rd_mask $rd_dir]
	}
	close $rd_f
}

set ::rd_fh [open $::rd_out w]
fconfigure $::rd_fh -translation binary
set ::rd_ev [open "$::rd_out.events" w]

proc rd_u32 {v} { return [binary format i $v] }

# H.KEYI hook target: one entry per interrupt.
proc rd_on_irq {} { incr ::rd_irq }

# Per-interrupt work: count it and place it by VRAM writes this tick.
proc rd_on_work {} {
	incr ::rd_work
	puts $::rd_ev "W [rd_evtick] $::rd_vw [format %04X [peek16 [reg SP]]]"
}

proc rd_evtick {} {
	if {!$::rd_started} { return -2 }
	return [expr {$::rd_tick - 1}]
}

# Every interrupt, with the interrupted PC (the return address IM 1 pushed).
proc rd_on_int {} {
	if {!$::rd_ininit && !$::rd_started} return
	puts $::rd_ev "I [rd_evtick] $::rd_vw [format %04X [peek16 [reg SP]]] [expr {[peek 0xE005] & 1}] $::rd_ss"
}

# INIT entry: start counting VRAM writes for the INIT-time interrupts.
proc rd_on_init {} {
	set ::rd_ininit 1
	set ::rd_vw 0
	set ::rd_ss 0
}

proc rd_on_sound {} { incr ::rd_ss }

# INIT's final EI: write the header.
proc rd_on_init_done {} {
	if {$::rd_started} return
	puts -nonewline $::rd_fh "CXTRACE1[rd_u32 $::rd_irq][rd_u32 $::rd_work]"
	set ::rd_started 1
	set ::rd_vw 0
	set ::rd_ss 0
}

# Tick-body start: apply input, then record.
proc rd_on_tick {} {
	if {!$::rd_started} return
	if {[dict exists $::rd_sched $::rd_tick]} {
		foreach rd_k [dict get $::rd_sched $::rd_tick] {
			lassign $rd_k rd_row rd_mask rd_dir
			if {$rd_dir eq "down"} {
				keymatrixdown $rd_row $rd_mask
			} else {
				keymatrixup $rd_row $rd_mask
			}
			puts $::rd_ev "K $::rd_tick $rd_row $rd_mask $rd_dir"
		}
	}
	puts -nonewline $::rd_fh [rd_u32 $::rd_tick][rd_u32 $::rd_irq][rd_u32 $::rd_work]
	puts -nonewline $::rd_fh [debug read_block "VDP regs" 0 8]
	puts -nonewline $::rd_fh [debug read_block "PSG regs" 0 16]
	puts -nonewline $::rd_fh [debug read_block memory 0xE000 0x1000]
	puts -nonewline $::rd_fh [debug read_block VRAM 0 0x4000]
	set ::rd_vw 0
	set ::rd_ss 0
	incr ::rd_tick
	if {$::rd_tick >= $::rd_ticks} {
		close $::rd_fh
		close $::rd_ev
		exit
	}
}

proc rd_on_vram_write {} { incr ::rd_vw }

# LD A,R at $93AE: log the value once the instruction has executed.
proc rd_on_ldar {} {
	puts $::rd_ev "R [expr {$::rd_tick - 1}] [reg A]"
}

set renderer none
set throttle off
set mute on

debug set_bp 0x407E {} rd_on_init
debug set_bp 0x0038 {} rd_on_int
debug set_bp 0x402E {} rd_on_irq
debug set_bp 0x490C {} rd_on_sound
debug set_bp 0x4901 {} rd_on_sound
foreach rd_a {0x41A4 0x420D 0x57E4 0x5BD4 0x79DC 0x8FA8} {
	debug set_bp $rd_a {} rd_on_sound
}
debug set_bp 0x49C0 {} rd_on_work
debug set_bp 0x40C8 {} rd_on_init_done
debug set_bp 0x403E {} rd_on_tick
debug set_bp 0x93B0 {} rd_on_ldar
debug set_watchpoint write_io 0x98 {} rd_on_vram_write
