import { PanelSection, PanelSectionRow, SliderField, ToggleField } from "@decky/ui";
import { useState } from "react";
import { check, setSettings } from "../backend";
import { ChargerSettings, StatusData } from "../types";

export function ChargerSection({ data }: { data: StatusData }) {
  const ch = data.charger;

  // local copy so sliders dont jump around while the 1s poll catches up
  const [cs, setCs] = useState<ChargerSettings>(ch.settings);

  const watts = cs.watts > 0 ? cs.watts : data.max;

  // same as the backend, full clock only from gpu_full_watts up
  const gpuCap = (w: number) => {
    const c = Math.floor((ch.gpu_max * w) / ch.gpu_full_watts / 100) * 100;
    return Math.max(ch.gpu_min, Math.min(ch.gpu_max, c));
  };
  const cap = gpuCap(watts);

  const update = async (patch: Partial<ChargerSettings>) => {
    const next = { ...cs, ...patch };
    // dropping the tdp pulls the clock down with it
    const c = gpuCap(next.watts > 0 ? next.watts : data.max);
    if (next.gpu_clock > c) {
      next.gpu_clock = c;
    }
    setCs(next);
    check(await setSettings(next));
  };

  return (
    <PanelSection title="On charger">
      <PanelSectionRow>
        <ToggleField
          label="Full power on charger"
          description={ch.boosted ? `Plugged in, running at ${ch.watts}W` : ch.ac ? "Plugged in" : "On battery"}
          checked={cs.full_power}
          onChange={(v) => update({ full_power: v })}
        />
      </PanelSectionRow>
      {cs.full_power && (
        <>
          <PanelSectionRow>
            <SliderField
              label="TDP on charger"
              value={watts}
              min={data.min}
              max={data.max}
              step={1}
              showValue
              valueSuffix="W"
              onChange={(v) => update({ watts: v })}
            />
          </PanelSectionRow>
          {ch.gpu_max > 0 && (
            <PanelSectionRow>
              <ToggleField label="Pin GPU clock" checked={cs.pin_gpu} onChange={(v) => update({ pin_gpu: v })} />
            </PanelSectionRow>
          )}
          {ch.gpu_max > 0 && cs.pin_gpu && (
            <PanelSectionRow>
              <SliderField
                label="GPU clock"
                description={cap < ch.gpu_max ? `Limited to ${cap}MHz at ${watts}W` : undefined}
                value={cs.gpu_clock > 0 ? Math.min(cs.gpu_clock, cap) : cap}
                min={ch.gpu_min}
                max={cap}
                step={100}
                showValue
                valueSuffix="MHz"
                onChange={(v) => update({ gpu_clock: v })}
              />
            </PanelSectionRow>
          )}
        </>
      )}
    </PanelSection>
  );
}
