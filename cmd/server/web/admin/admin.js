const endpoints = {
  maxLogin: "/api/v1/auth/max",
  maxIdentity: "/api/v1/auth/max/identity",
  pendingSubmissions: "/api/v1/admin/submissions",
};

const moderatorTokenKey = "botshelf-moderator-token";
const adminInitDataKey = "botshelf-admin-init-data";
const adminAccess = document.querySelector("#admin-access");
const accessMessage = document.querySelector("#access-message");
const moderation = document.querySelector("#moderation");
const moderationMessage = document.querySelector("#moderation-message");
const moderationList = document.querySelector("#moderation-list");
const moderationTemplate = document.querySelector("#moderation-card-template");
const refreshModeration = document.querySelector("#refresh-moderation");
const moderatorLogout = document.querySelector("#moderator-logout");

async function getJSON(url, options = {}) {
  const response = await fetch(url, options);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(body.error || "Сервис временно недоступен");
    error.status = response.status;
    throw error;
  }
  return body;
}

function moderatorHeaders() {
  const token = sessionStorage.getItem(moderatorTokenKey);
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function showModeration(message = "") {
  adminAccess.hidden = true;
  moderation.hidden = false;
  moderationMessage.textContent = message;
}

function showAccess(message) {
  moderation.hidden = true;
  adminAccess.hidden = false;
  accessMessage.textContent = message;
}

function formatDate(value) {
  return new Intl.DateTimeFormat("ru-RU", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

async function loadModerationQueue() {
  if (!sessionStorage.getItem(moderatorTokenKey)) return;
  showModeration("Загружаем очередь…");
  moderationList.replaceChildren();
  try {
    const data = await getJSON(endpoints.pendingSubmissions, { headers: moderatorHeaders() });
    moderationMessage.textContent = data.total ? `Заявок на проверке: ${data.total}` : "Очередь пуста.";
    for (const submission of data.items) renderModerationCard(submission);
  } catch (error) {
    if (error.status === 401) {
      sessionStorage.removeItem(moderatorTokenKey);
      showAccess("Сессия закончилась. Снова откройте служебный диплинк в MAX.");
      return;
    }
    moderationMessage.textContent = error.message;
  }
}

function renderModerationCard(submission) {
  const fragment = moderationTemplate.content.cloneNode(true);
  fragment.querySelector("h3").textContent = submission.name;
  fragment.querySelector(".username").textContent = `@${submission.username}`;
  fragment.querySelector(".description").textContent = submission.description;
  fragment.querySelector(".badge").textContent = submission.category;
  fragment.querySelector(".submitted-at").textContent = `Отправлено: ${formatDate(submission.created_at)}`;
  fragment.querySelector(".submission-source").href = submission.source_url;
  fragment.querySelector(".owner-contact").textContent = submission.owner_contact;
  fragment.querySelector(".approve").addEventListener("click", () => moderate(submission.id, "approve"));
  fragment.querySelector(".reject").addEventListener("click", () => {
    const note = window.prompt("Почему заявка не подходит?");
    if (note !== null) moderate(submission.id, "reject", note);
  });
  moderationList.append(fragment);
}

async function moderate(id, action, note = "") {
  moderationMessage.textContent = action === "approve" ? "Публикуем карточку…" : "Отклоняем заявку…";
  try {
    await getJSON(`${endpoints.pendingSubmissions}/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...moderatorHeaders() },
      body: action === "reject" ? JSON.stringify({ note }) : undefined,
    });
    await loadModerationQueue();
  } catch (error) {
    moderationMessage.textContent = error.message;
  }
}

async function loginFromMAX() {
  const initData = sessionStorage.getItem(adminInitDataKey) || window.WebApp?.initData;
  if (!initData) {
    showAccess("Панель нужно открыть внутри MAX по диплинку этого бота: https://max.ru/<ник_бота>?startapp=admin");
    return;
  }
  showAccess("Проверяем подпись MAX и права модератора…");
  try {
    const data = await getJSON(endpoints.maxLogin, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ init_data: initData }),
    });
    sessionStorage.setItem(moderatorTokenKey, data.token);
    await loadModerationQueue();
  } catch (error) {
    if (error.status === 403) {
      try {
        const identity = await getJSON(endpoints.maxIdentity, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ init_data: initData }),
        });
        showAccess(`Ваш MAX ID: ${identity.user_id}. Добавьте его в MODERATOR_MAX_USER_IDS и перезапустите сервис.`);
        return;
      } catch {
        // The original error is more useful than a secondary identity lookup failure.
      }
    }
    showAccess(error.message);
  } finally {
    // initData is signed but it is still one-time launch data, not a session credential.
    sessionStorage.removeItem(adminInitDataKey);
  }
}

refreshModeration.addEventListener("click", loadModerationQueue);
moderatorLogout.addEventListener("click", () => {
  sessionStorage.removeItem(moderatorTokenKey);
  sessionStorage.removeItem(adminInitDataKey);
  showAccess("Вы вышли. Для нового входа откройте служебный диплинк в MAX.");
});

(async () => {
  window.WebApp?.ready?.();
  if (sessionStorage.getItem(moderatorTokenKey)) {
    await loadModerationQueue();
    return;
  }
  await loginFromMAX();
})();
