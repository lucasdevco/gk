import type { Task, Error as ErrorResponse } from "./api-client/types.gen";

type TaskList = { items: Task[] };

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    const body = (await response
      .json()
      .catch(() => null)) as ErrorResponse | null;
    throw new Error(
      body?.error?.message ?? `Request failed (${response.status})`,
    );
  }
  return response.json() as Promise<T>;
}

export const api = {
  listTasks: () => request<TaskList>("/api/v1/tasks"),
  createTask: (title: string) =>
    request<Task>("/api/v1/tasks", {
      method: "POST",
      body: JSON.stringify({ title }),
    }),
  updateTask: (id: string, completed: boolean) =>
    request<Task>(`/api/v1/tasks/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ completed }),
    }),
};
