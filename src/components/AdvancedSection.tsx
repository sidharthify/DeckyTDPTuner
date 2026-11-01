import { ButtonItem, Field, PanelSection, PanelSectionRow, SliderField, ToggleField } from "@decky/ui";
import { useState } from "react";
import { check, setSettings } from "../backend";
import { StatusData } from "../types";

const warn = { color: "#ff6b6b", fontWeight: "bold" };

export function AdvancedSection({ data }: { data: StatusData }) {
  const ch = data.charger;
  const [open, setOpen] = useState(false);
  const [floor, setFloor] = useState(ch.floor);

  const update = async (minWatts: number) => {
    setFloor(minWatts > 0 ? minWatts : ch.default_min);
    check(await setSettings({ min_watts: minWatts }));
  };

  return (
    <PanelSection title="Advanced">
      <PanelSectionRow>
        <ToggleField label="Show advanced settings" checked={open} onChange={setOpen} />
      </PanelSectionRow>
      {open && (
        <>
          <PanelSectionRow>
            <Field
              label={<span style={warn}>Can crash your game</span>}
              description={`Some games crash or hang outright at very low TDP (Geometry Dash does at 3W). A lower power cap won't hurt the Deck itself, but a hung game can lose unsaved progress. ${ch.default_min}W is the default for a reason.`}
            />
          </PanelSectionRow>
          <PanelSectionRow>
            <SliderField
              label="Lowest TDP"
              description="Tuning and live mode never go under this"
              value={floor}
              min={data.min}
              max={Math.min(10, data.max)}
              step={1}
              showValue
              valueSuffix="W"
              onChange={update}
            />
          </PanelSectionRow>
          {floor != ch.default_min && (
            <PanelSectionRow>
              <ButtonItem layout="below" onClick={() => update(0)}>
                Back to default ({ch.default_min}W)
              </ButtonItem>
            </PanelSectionRow>
          )}
        </>
      )}
    </PanelSection>
  );
}
