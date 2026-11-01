import { PanelSection, PanelSectionRow, SliderField, ToggleField } from "@decky/ui";
import { useState } from "react";
import { check, setSettings } from "../backend";
import { ChargerSettings, StatusData } from "../types";

export function ChargerSection({ data }: { data: StatusData }) {
  const ch = data.charger;

  // local copy so sliders dont jump around while the 1s poll catches up
  const [cs, setCs] = useState<ChargerSettings>(ch.settings);

  const update = async (patch: Partial<ChargerSettings>) => {
    const next = { ...cs, ...patch };
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
              value={cs.watts > 0 ? cs.watts : data.max}
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
                value={cs.gpu_clock > 0 ? cs.gpu_clock : ch.gpu_max}
                min={ch.gpu_min}
                max={ch.gpu_max}
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
