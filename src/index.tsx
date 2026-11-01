import { staticClasses } from "@decky/ui";
import { definePlugin } from "@decky/api";
import { FaBolt } from "react-icons/fa";
import { Content } from "./components/Content";
import { watchGames } from "./lifetime";

export default definePlugin(() => {
  const lifetime = watchGames();

  return {
    name: "TDP Tuner",
    titleView: <div className={staticClasses.Title}>TDP Tuner</div>,
    content: <Content />,
    icon: <FaBolt />,
    onDismount() {
      lifetime.unregister();
    },
  };
});
