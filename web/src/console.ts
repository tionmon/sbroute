import type { Dispatch, SetStateAction } from "react";
import type {
  Config,
  Event,
  NodeDef,
  Resources,
  Revision,
  Stats,
  SubStatus,
} from "./types";

export type ModalState = { kind: string; value?: unknown };
export type Removable = "nodes" | "groups" | "entrances" | "subscriptions";

// Console is the shared state and actions every page receives from App once the configuration is loaded.
export type Console = {
  cfg: Config;
  stats?: Stats;
  busy: boolean;
  error: string;
  action: (fn: () => Promise<void>) => Promise<void>;
  save: (next: Config) => Promise<void>;
  modal?: ModalState;
  setModal: (modal?: ModalState) => void;
  load: () => Promise<void>;
  tell: (message: string) => void;
  changePage: (key: string) => void;
  remove: (type: Removable, itemID: string) => Promise<void>;
  openImport: () => void;
  refreshStats: () => Promise<void>;
  points: number[];
  events: Event[];
  subStatus: SubStatus[];
  presets: Resources[];
  revisions: Revision[];
  search: string;
  setSearch: (value: string) => void;
  nodes: NodeDef[];
  visibleNodes: NodeDef[];
  nodePage: number;
  setNodePage: Dispatch<SetStateAction<number>>;
  setAuthenticated: (value: boolean) => void;
};
