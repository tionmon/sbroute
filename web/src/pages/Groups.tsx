import { ArrowRight, Layers, Plus, Trash2 } from "lucide-react";
import { Empty } from "../ui";
import type { Console } from "../console";

export function Groups({ ctx }: { ctx: Console }) {
  const { cfg, action, setModal, remove } = ctx;
  return cfg.groups.length ? (
    <div className="group-grid">
      {cfg.groups.map((g) => {
        const manual = (g.members || []).length;
        return (
          <section className="panel group-card" key={g.id}>
            <div className="group-card-top">
              <span className="route-icon">
                <Layers size={22} />
              </span>
              <button
                className="icon-button danger"
                aria-label="删除分组"
                onClick={() => action(() => remove("groups", g.id))}
              >
                <Trash2 size={16} />
              </button>
            </div>
            <h2>{g.name}</h2>
            <p>
              {manual} 个手动成员 · {(g.sources || []).length} 个订阅来源
            </p>
            <div className="group-rules">
              {g.keyword && <code>包含：{g.keyword}</code>}
              {g.regex && <code>正则：{g.regex}</code>}
              {g.exclude && <code>排除：{g.exclude}</code>}
              {!g.keyword && !g.regex && !(g.sources || []).length && (
                <span>手动选择与优先级排序</span>
              )}
            </div>
            <footer>
              <span>
                {cfg.entrances.filter((e) => e.group_id === g.id).length}{" "}
                个入口引用
              </span>
              <button
                className="text-button"
                onClick={() => setModal({ kind: "group", value: g })}
              >
                编辑分组 <ArrowRight size={15} />
              </button>
            </footer>
          </section>
        );
      })}
    </div>
  ) : (
    <section className="panel">
      <Empty
        title="将节点组织成组"
        text="按落地地区、订阅来源或用途建立分组，多个入口可共享同一组。"
        action={
          <button
            className="button primary"
            onClick={() => setModal({ kind: "group" })}
          >
            <Plus size={16} />
            新建分组
          </button>
        }
      />
    </section>
  );
}
