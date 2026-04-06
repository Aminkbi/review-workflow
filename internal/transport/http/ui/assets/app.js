const state = {
  actor: { id: "alice", role: "employee" },
  requests: [],
  selectedRequestId: "",
};

const elements = {
  actorDisplay: document.getElementById("actor-display"),
  createForm: document.getElementById("create-form"),
  requestList: document.getElementById("request-list"),
  statusFilter: document.getElementById("status-filter"),
  refreshButton: document.getElementById("refresh-button"),
  detailEmpty: document.getElementById("detail-empty"),
  detailContent: document.getElementById("detail-content"),
  detailTitle: document.getElementById("detail-title"),
  detailStatus: document.getElementById("detail-status"),
  detailGrid: document.getElementById("detail-grid"),
  auditList: document.getElementById("audit-list"),
  submitAction: document.getElementById("submit-action"),
  approveAction: document.getElementById("approve-action"),
  rejectAction: document.getElementById("reject-action"),
  reviewComment: document.getElementById("review-comment"),
  toast: document.getElementById("toast"),
  statTotal: document.getElementById("stat-total"),
  statPending: document.getElementById("stat-pending"),
  statFailed: document.getElementById("stat-failed"),
};

function syncActorButtons() {
  document.querySelectorAll(".persona-card").forEach((card) => {
    const isActive = card.dataset.actorId === state.actor.id && card.dataset.actorRole === state.actor.role;
    card.classList.toggle("active", isActive);
  });
  elements.actorDisplay.textContent = `${state.actor.id} / ${state.actor.role}`;
}

function hydrateFromQuery() {
  const params = new URLSearchParams(window.location.search);
  const actorId = params.get("actor_id");
  const actorRole = params.get("actor_role");
  const status = params.get("status");

  if (actorId && actorRole) {
    state.actor = { id: actorId, role: actorRole };
  }
  if (status) {
    elements.statusFilter.value = status;
  }
  syncActorButtons();
}

function actorHeaders(extra = {}) {
  return {
    "X-Actor-Id": state.actor.id,
    "X-Actor-Role": state.actor.role,
    ...extra,
  };
}

async function api(path, options = {}) {
  const response = await fetch(path, options);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `Request failed with ${response.status}`);
  }
  return data;
}

function showToast(message, isError = false) {
  elements.toast.textContent = message;
  elements.toast.style.background = isError ? "rgba(166, 58, 47, 0.95)" : "rgba(31, 26, 23, 0.95)";
  elements.toast.classList.remove("hidden");
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => elements.toast.classList.add("hidden"), 2800);
}

function formatDate(value) {
  if (!value) return "n/a";
  return new Date(value).toLocaleString();
}

function idempotencyKey(prefix, requestId = "") {
  return `${prefix}-${requestId}-${Date.now()}`;
}

function renderStats() {
  elements.statTotal.textContent = String(state.requests.length);
  elements.statPending.textContent = String(state.requests.filter((item) => item.status === "pending_review").length);
  elements.statFailed.textContent = String(state.requests.filter((item) => item.status === "execution_failed").length);
}

function renderRequests() {
  if (!state.requests.length) {
    elements.requestList.innerHTML = `<div class="empty-state">No requests are visible for the current actor and filter.</div>`;
    renderStats();
    return;
  }

  elements.requestList.innerHTML = state.requests.map((item) => `
    <article class="request-card ${item.id === state.selectedRequestId ? "active" : ""}" data-request-id="${item.id}">
      <div class="request-topline">
        <span class="detail-label">${item.type}</span>
        <span class="status-chip status-${item.status}">${item.status}</span>
      </div>
      <h3>${item.target_resource}</h3>
      <p class="request-meta">${item.justification}</p>
      <div class="request-footer">
        <span>Requester ${item.requester_id}</span>
        <span>v${item.version}</span>
      </div>
    </article>
  `).join("");

  elements.requestList.querySelectorAll("[data-request-id]").forEach((card) => {
    card.addEventListener("click", () => {
      state.selectedRequestId = card.dataset.requestId;
      renderRequests();
      loadSelectedRequest();
    });
  });
  renderStats();
}

function renderDetail(request, audit) {
  if (!request) {
    elements.detailEmpty.classList.remove("hidden");
    elements.detailContent.classList.add("hidden");
    return;
  }

  elements.detailEmpty.classList.add("hidden");
  elements.detailContent.classList.remove("hidden");
  elements.detailTitle.textContent = `${request.type} · ${request.target_resource}`;
  elements.detailStatus.textContent = request.status;
  elements.detailStatus.className = `status-chip status-${request.status}`;

  const details = [
    ["Request ID", request.id],
    ["Requester", request.requester_id],
    ["Assigned reviewer", request.assigned_reviewer_id || "unassigned"],
    ["Justification", request.justification],
    ["Created", formatDate(request.created_at)],
    ["Submitted", formatDate(request.submitted_at)],
    ["Reviewed", formatDate(request.reviewed_at)],
    ["Executed", formatDate(request.executed_at)],
    ["Reminder count", String(request.reminder_count)],
    ["Execution attempts", String(request.execution_attempts)],
    ["Last execution error", request.last_execution_error || "none"],
    ["Version", String(request.version)],
  ];

  elements.detailGrid.innerHTML = details.map(([label, value]) => `
    <div>
      <dt>${label}</dt>
      <dd>${value}</dd>
    </div>
  `).join("");

  elements.auditList.innerHTML = audit.length ? audit.map((entry) => `
    <article class="audit-entry">
      <strong>${entry.action}</strong>
      <p>${entry.description}</p>
      <span class="detail-label">${entry.actor_id} / ${entry.actor_role}</span>
      <time>${formatDate(entry.created_at)}</time>
    </article>
  `).join("") : `<div class="empty-state">No audit entries found for this request.</div>`;

  const canSubmit = request.status === "draft" && request.requester_id === state.actor.id;
  const canReview = request.status === "pending_review" &&
    (state.actor.role === "admin" || (state.actor.role === "reviewer" && request.assigned_reviewer_id === state.actor.id && request.requester_id !== state.actor.id));

  elements.submitAction.disabled = !canSubmit;
  elements.approveAction.disabled = !canReview;
  elements.rejectAction.disabled = !canReview;
}

async function loadRequests() {
  const query = new URLSearchParams();
  if (elements.statusFilter.value) {
    query.set("status", elements.statusFilter.value);
  }
  const data = await api(`/v1/requests?${query.toString()}`, {
    headers: actorHeaders(),
  });
  state.requests = data.items || [];
  if (state.selectedRequestId && !state.requests.some((item) => item.id === state.selectedRequestId)) {
    state.selectedRequestId = "";
  }
  if (!state.selectedRequestId && state.requests.length) {
    state.selectedRequestId = state.requests[0].id;
  }
  renderRequests();
  if (state.selectedRequestId) {
    await loadSelectedRequest();
  } else {
    renderDetail(null, []);
  }
}

async function loadSelectedRequest() {
  if (!state.selectedRequestId) {
    renderDetail(null, []);
    return;
  }
  const [request, auditData] = await Promise.all([
    api(`/v1/requests/${state.selectedRequestId}`, { headers: actorHeaders() }),
    api(`/v1/requests/${state.selectedRequestId}/audit`, { headers: actorHeaders() }),
  ]);
  renderDetail(request, auditData.items || []);
}

async function createDraft(event) {
  event.preventDefault();
  const formData = new FormData(elements.createForm);
  const payload = Object.fromEntries(formData.entries());
  const created = await api("/v1/requests", {
    method: "POST",
    headers: {
      ...actorHeaders(),
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });
  elements.createForm.reset();
  state.selectedRequestId = created.id;
  showToast("Draft created");
  await loadRequests();
}

async function submitSelected() {
  if (!state.selectedRequestId) return;
  await api(`/v1/requests/${state.selectedRequestId}/submit`, {
    method: "POST",
    headers: actorHeaders({
      "Idempotency-Key": idempotencyKey("submit", state.selectedRequestId),
    }),
  });
  showToast("Request submitted for review");
  await loadRequests();
}

async function reviewSelected(decision) {
  if (!state.selectedRequestId) return;
  await api(`/v1/requests/${state.selectedRequestId}/${decision}`, {
    method: "POST",
    headers: {
      ...actorHeaders({
        "Idempotency-Key": idempotencyKey(decision, state.selectedRequestId),
      }),
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ comment: elements.reviewComment.value.trim() }),
  });
  elements.reviewComment.value = "";
  showToast(`Request ${decision}d`);
  await loadRequests();
}

function selectActor(button) {
  state.actor = {
    id: button.dataset.actorId,
    role: button.dataset.actorRole,
  };
  syncActorButtons();
  state.selectedRequestId = "";
  loadRequests().catch((error) => showToast(error.message, true));
}

function bindEvents() {
  elements.createForm.addEventListener("submit", (event) => {
    createDraft(event).catch((error) => showToast(error.message, true));
  });
  elements.statusFilter.addEventListener("change", () => {
    loadRequests().catch((error) => showToast(error.message, true));
  });
  elements.refreshButton.addEventListener("click", () => {
    loadRequests().catch((error) => showToast(error.message, true));
  });
  elements.submitAction.addEventListener("click", () => {
    submitSelected().catch((error) => showToast(error.message, true));
  });
  elements.approveAction.addEventListener("click", () => {
    reviewSelected("approve").catch((error) => showToast(error.message, true));
  });
  elements.rejectAction.addEventListener("click", () => {
    reviewSelected("reject").catch((error) => showToast(error.message, true));
  });
  document.querySelectorAll(".persona-card").forEach((button) => {
    button.addEventListener("click", () => selectActor(button));
  });
}

hydrateFromQuery();
bindEvents();
loadRequests().catch((error) => showToast(error.message, true));
