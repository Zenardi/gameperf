package collector_test

import (
	"strings"
	"testing"

	"github.com/zenardi/gameperf/internal/collector"
)

// Realistic /proc/interrupts excerpt with 4 CPUs.
const procInterrupts4CPU = `           CPU0       CPU1       CPU2       CPU3
  9:          0          0          0       1746762  IR-IO-APIC   9-fasteoi   acpi
217:     116647          0          0          0  PCI-MSI 524800-edge      nvidia
243:          0          0    5000000    9068284  PCI-MSI 327680-edge      i915
LOC:  12345678   23456789   34567890   45678901  Local timer interrupts
`

func TestParseIRQs_EntryCount(t *testing.T) {
	entries, cpuCount, err := collector.ParseIRQs(strings.NewReader(procInterrupts4CPU))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpuCount != 4 {
		t.Errorf("cpuCount = %d, want 4", cpuCount)
	}
	if len(entries) != 4 {
		t.Errorf("len(entries) = %d, want 4", len(entries))
	}
}

func TestParseIRQs_NvidiaEntry(t *testing.T) {
	entries, _, err := collector.ParseIRQs(strings.NewReader(procInterrupts4CPU))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var nvidia *collector.IRQEntry
	for i := range entries {
		if entries[i].Number == "217" {
			nvidia = &entries[i]
			break
		}
	}
	if nvidia == nil {
		t.Fatal("IRQ 217 (nvidia) not found")
	}
	if nvidia.Total != 116647 {
		t.Errorf("Total = %d, want 116647", nvidia.Total)
	}
	if !strings.Contains(nvidia.Name, "nvidia") {
		t.Errorf("Name = %q, want to contain 'nvidia'", nvidia.Name)
	}
}

func TestParseIRQs_TotalIsSum(t *testing.T) {
	entries, _, err := collector.ParseIRQs(strings.NewReader(procInterrupts4CPU))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entries {
		var sum int64
		for _, v := range e.PerCPU {
			sum += v
		}
		if sum != e.Total {
			t.Errorf("IRQ %s: Total=%d but sum(PerCPU)=%d", e.Number, e.Total, sum)
		}
	}
}

func TestIRQEntry_TopCPU(t *testing.T) {
	// i915: CPU2=5000000, CPU3=9068284 → top is CPU3 (index 3)
	entries, _, err := collector.ParseIRQs(strings.NewReader(procInterrupts4CPU))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var i915 *collector.IRQEntry
	for i := range entries {
		if entries[i].Number == "243" {
			i915 = &entries[i]
			break
		}
	}
	if i915 == nil {
		t.Fatal("IRQ 243 (i915) not found")
	}
	if top := i915.TopCPU(); top != 3 {
		t.Errorf("TopCPU() = %d, want 3", top)
	}
}

func TestFindIRQByName_CaseInsensitive(t *testing.T) {
	entries, _, _ := collector.ParseIRQs(strings.NewReader(procInterrupts4CPU))

	found := collector.FindIRQByName(entries, "NVIDIA")
	if len(found) != 1 {
		t.Errorf("FindIRQByName(NVIDIA) = %d results, want 1", len(found))
	}
	if len(found) > 0 && found[0].Number != "217" {
		t.Errorf("found IRQ %s, want 217", found[0].Number)
	}
}

func TestFindIRQByName_NoMatch(t *testing.T) {
	entries, _, _ := collector.ParseIRQs(strings.NewReader(procInterrupts4CPU))
	found := collector.FindIRQByName(entries, "amdgpu")
	if len(found) != 0 {
		t.Errorf("expected no results, got %d", len(found))
	}
}

func TestParseIRQs_EmptyInput(t *testing.T) {
	entries, cpuCount, err := collector.ParseIRQs(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpuCount != 0 {
		t.Errorf("cpuCount = %d, want 0", cpuCount)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %d", len(entries))
	}
}
