const endpoints = {
  bots: "/api/v1/bots",
  categories: "/api/v1/categories",
  submissions: "/api/v1/submissions",
};

const searchInput = document.querySelector("#search");
const categorySelect = document.querySelector("#category");
const results = document.querySelector("#results");
const resultsMeta = document.querySelector("#results-meta");
const cardTemplate = document.querySelector("#bot-card-template");
const form = document.querySelector("#submission-form");
const formMessage = document.querySelector("#form-message");
const adminInitDataKey = "botshelf-admin-init-data";

let categories = [];
let searchTimer;

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

async function loadCategories() {
  const data = await getJSON(endpoints.categories);
  categories = data.categories;
  for (const category of categories) {
    categorySelect.add(new Option(category, category));
    document.querySelector("#submission-category").add(new Option(category, category));
  }
}

async function loadBots() {
  const params = new URLSearchParams();
  if (searchInput.value.trim()) params.set("q", searchInput.value.trim());
  if (categorySelect.value) params.set("category", categorySelect.value);

  results.replaceChildren();
  resultsMeta.textContent = "Ищем…";
  try {
    const data = await getJSON(`${endpoints.bots}?${params}`);
    resultsMeta.textContent = `Найдено: ${data.total}`;
    if (data.total === 0) {
      results.innerHTML = "<p class=\"empty\">Пока ничего не найдено. Попробуйте другой запрос или добавьте бота в каталог.</p>";
      return;
    }
    for (const bot of data.items) renderBot(bot);
  } catch (error) {
    resultsMeta.textContent = error.message;
  }
}

function renderBot(bot) {
  const fragment = cardTemplate.content.cloneNode(true);
  fragment.querySelector("h2").textContent = bot.name;
  fragment.querySelector(".username").textContent = `@${bot.username}`;
  fragment.querySelector(".description").textContent = bot.description;
  fragment.querySelector(".badge").textContent = bot.category;
  fragment.querySelector(".verification-date").textContent = `Проверено: ${new Intl.DateTimeFormat("ru-RU", { dateStyle: "medium" }).format(new Date(bot.last_verified_at))}`;

  const openLink = fragment.querySelector(".open-bot");
  openLink.href = `https://max.ru/${encodeURIComponent(bot.username)}`;
  openLink.addEventListener("click", (event) => openMaxLinkInApp(event, openLink.href));
  fragment.querySelector(".source").href = bot.source_url;
  results.append(fragment);
}

// Outside MAX the ordinary HTTPS link remains usable. Inside a mini-app the
// official bridge keeps navigation inside MAX instead of opening a browser.
// initDataUnsafe is deliberately not used for authentication: the platform
// documentation says it is not validated server-side data.
function openMaxLinkInApp(event, url) {
  if (!window.WebApp || typeof window.WebApp.openMaxLink !== "function") return;
  event.preventDefault();
  window.WebApp.openMaxLink(url);
}

function delayedSearch() {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(loadBots, 250);
}

function startParameter() {
  const value = window.WebApp?.initDataUnsafe?.start_param;
  if (typeof value === "string") return value;
  if (value && typeof value.value === "string") return value.value;
  return "";
}

function openAdminPanelFromDeepLink() {
  // start_param selects a page only. Authorization happens later on the server
  // after validation of the signed initData carried into the same browser tab.
  if (startParameter() !== "admin") return false;
  const initData = window.WebApp?.initData;
  if (initData) sessionStorage.setItem(adminInitDataKey, initData);
  window.location.replace("/admin/");
  return true;
}

searchInput.addEventListener("input", delayedSearch);
categorySelect.addEventListener("change", loadBots);

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  formMessage.textContent = "Отправляем…";
  const values = Object.fromEntries(new FormData(form).entries());
  try {
    await getJSON(endpoints.submissions, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
    });
    form.reset();
    formMessage.textContent = "Заявка принята. Она появится в каталоге только после проверки.";
  } catch (error) {
    formMessage.textContent = error.message;
  }
});

(async () => {
  try {
    window.WebApp?.ready?.();
    if (openAdminPanelFromDeepLink()) return;
    await loadCategories();
    await loadBots();
  } catch (error) {
    resultsMeta.textContent = error.message;
  }
})();
