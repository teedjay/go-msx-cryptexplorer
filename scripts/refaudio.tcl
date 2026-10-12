# Record the original's audio with openMSX (C-BIOS_MSX1_JP), for
# cmd/audiocmp. Parameters from the environment:
#   RA_OUT    WAV file to write (outside the repo: it is ROM-derived)
#   RA_TICKS  stop after this many game ticks (default 3000)
#   RA_KEYS   optional key schedule, lines "tick:row:mask:down|up"
# Recording starts at INIT's entry ($406C), stops at the tick limit.
proc ra_env {name def} {
	if {[info exists ::env($name)]} { return $::env($name) }
	return $def
}
set ::ra_out [ra_env RA_OUT ""]
set ::ra_ticks [ra_env RA_TICKS 3000]
set ::ra_tick 0
set ::ra_sched [dict create]
if {[set ::ra_keys [ra_env RA_KEYS ""]] ne ""} {
	set ra_f [open $::ra_keys r]
	foreach ra_line [split [read $ra_f] "\n"] {
		set ra_line [string trim $ra_line]
		if {$ra_line eq "" || [string index $ra_line 0] eq "#"} continue
		lassign [split $ra_line ":"] ra_t ra_row ra_mask ra_dir
		dict lappend ::ra_sched $ra_t [list $ra_row $ra_mask $ra_dir]
	}
	close $ra_f
}
proc ra_on_init {} { record start -audioonly $::ra_out }
proc ra_on_tick {} {
	if {[dict exists $::ra_sched $::ra_tick]} {
		foreach ra_k [dict get $::ra_sched $::ra_tick] {
			lassign $ra_k ra_row ra_mask ra_dir
			if {$ra_dir eq "down"} { keymatrixdown $ra_row $ra_mask } else { keymatrixup $ra_row $ra_mask }
		}
	}
	incr ::ra_tick
	if {$::ra_tick > $::ra_ticks} {
		record stop
		exit
	}
}
set renderer none
set throttle off
debug set_bp 0x406C {} ra_on_init
debug set_bp 0x402A {} ra_on_tick
