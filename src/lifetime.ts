import { toaster } from "@decky/api";
import { apply, reset, stop } from "./backend";

// apply the saved tdp when a game starts, put things back when it exits
export function watchGames() {
  return SteamClient.GameSessions.RegisterForAppLifetimeNotifications(async (n) => {
    const appid = String(n.unAppID);
    if (!n.bRunning) {
      await stop();
      await reset();
      return;
    }

    const r = await apply(appid);
    if (r.ok && r.data) {
      toaster.toast({ title: "TDP Tuner", body: `Set ${r.data.watts}W (tuned for ${r.data.target} fps)` });
    }
  });
}
