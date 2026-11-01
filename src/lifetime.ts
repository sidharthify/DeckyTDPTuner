import { Router } from "@decky/ui";
import { toaster } from "@decky/api";
import { apply, reset, stop } from "./backend";

let current = "";

async function applyFor(appid: string) {
  current = appid;
  const r = await apply(appid);
  if (r.ok && r.data) {
    toaster.toast({ title: "TDP Tuner", body: `Set ${r.data.watts}W (tuned for ${r.data.target} fps)` });
  }
}

// apply the saved tdp when a game starts, put things back when it exits
export function watchGames() {
  // plugin got (re)loaded mid game, the backend doesnt know about it yet
  const app = Router.MainRunningApp;
  if (app) {
    applyFor(String(app.appid));
  }

  return SteamClient.GameSessions.RegisterForAppLifetimeNotifications(async (n) => {
    if (!n.bRunning) {
      const appid = String(n.unAppID);
      if (n.unAppID && current && appid != current) {
        return;
      }
      current = "";
      await stop();
      await reset();
      return;
    }
    await applyFor(String(n.unAppID));
  });
}
