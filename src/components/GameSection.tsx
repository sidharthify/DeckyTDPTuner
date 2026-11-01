import { Field, PanelSection, PanelSectionRow } from "@decky/ui";
import { StatusData } from "../types";

type Props = {
  name?: string;
  data?: StatusData;
};

export function GameSection({ name, data }: Props) {
  const st = data?.status;

  return (
    <PanelSection title={name ?? "No game running"}>
      <PanelSectionRow>
        <Field label="Current TDP">{st ? `${st.watts}W` : "..."}</Field>
      </PanelSectionRow>
      {data?.profile && (
        <PanelSectionRow>
          <Field label="Saved" description={`tuned for ${data.profile.target} fps`}>
            {data.profile.watts}W
          </Field>
        </PanelSectionRow>
      )}
    </PanelSection>
  );
}
