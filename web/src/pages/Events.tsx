import { EventList } from "../ui";
import type { Console } from "../console";

export function Events({ ctx }: { ctx: Console }) {
  const { events } = ctx;
  return (
    <section className="panel">
      <div className="table-toolbar">
        <span>
          <i className="live-dot" /> 实时事件
        </span>
        <span className="subtle">最多展示最近 100 条</span>
      </div>
      <EventList events={events} />
    </section>
  );
}
