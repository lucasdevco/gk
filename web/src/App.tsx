import { FormEvent, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./lib/api";

const tasksKey = ["tasks"];

export function App() {
  const [title, setTitle] = useState("");
  const queryClient = useQueryClient();
  const tasks = useQuery({ queryKey: tasksKey, queryFn: api.listTasks });
  const createTask = useMutation({
    mutationFn: api.createTask,
    onSuccess: () => {
      setTitle("");
      void queryClient.invalidateQueries({ queryKey: tasksKey });
    },
  });
  const updateTask = useMutation({
    mutationFn: ({ id, completed }: { id: string; completed: boolean }) =>
      api.updateTask(id, completed),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: tasksKey }),
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (title.trim()) createTask.mutate(title);
  }

  return (
    <main className="mx-auto min-h-screen max-w-3xl px-6 py-16">
      <header className="mb-10">
        <span className="eyebrow">GO + REACT STARTER</span>
        <h1 className="mt-4 text-5xl font-semibold tracking-tight text-slate-950">
          Build the useful part.
        </h1>
        <p className="mt-4 max-w-xl text-lg leading-8 text-slate-600">
          GK 已经接好了数据库、类型安全 SQL、结构化日志、OpenTelemetry
          和单二进制前端。
        </p>
      </header>

      <section className="panel">
        <form className="flex gap-3" onSubmit={submit}>
          <input
            aria-label="任务标题"
            className="task-input"
            maxLength={200}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="下一件要完成的事"
            value={title}
          />
          <button
            className="primary-button"
            disabled={createTask.isPending}
            type="submit"
          >
            {createTask.isPending ? "添加中" : "添加"}
          </button>
        </form>

        {(tasks.error || createTask.error || updateTask.error) && (
          <p className="mt-4 text-sm text-red-600">
            {(tasks.error || createTask.error || updateTask.error)?.message}
          </p>
        )}

        <div className="mt-7 space-y-3">
          {tasks.isLoading && <p className="empty">正在加载…</p>}
          {tasks.data?.items.length === 0 && (
            <p className="empty">还没有任务，先创建一个。</p>
          )}
          {tasks.data?.items.map((task) => (
            <label className="task-row" key={task.id}>
              <input
                checked={task.completed}
                className="task-check"
                onChange={() =>
                  updateTask.mutate({ id: task.id, completed: !task.completed })
                }
                type="checkbox"
              />
              <span
                className={
                  task.completed
                    ? "text-slate-400 line-through"
                    : "text-slate-800"
                }
              >
                {task.title}
              </span>
            </label>
          ))}
        </div>
      </section>
    </main>
  );
}
