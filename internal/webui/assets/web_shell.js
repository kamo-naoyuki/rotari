// The page frame shared by the application and Job activity pages, inlined
// in <head> of both: the viewer's theme, the Settings menu, the navigation
// drawer on narrow screens, the sidebar's width, and why notifications are
// unavailable.
//
// The theme is "light", "dark", or "system" to follow the OS. It is kept per
// browser and applied before the page is drawn, so a stored choice never
// flashes the other theme. web_tokens.css reads it from data-theme on <html>.
(() => {
  const key = "rotari-theme";
  const choices = ["system", "light", "dark"];
  const stored = () => {
    try {
      const value = localStorage.getItem(key);
      return choices.includes(value) ? value : "system";
    } catch {
      return "system";
    }
  };
  const apply = (choice) => {
    if (choice === "light" || choice === "dark") {
      document.documentElement.dataset.theme = choice;
    } else {
      delete document.documentElement.dataset.theme;
    }
  };
  apply(stored());

  // The Settings menu opens from its button and closes on Escape or a click
  // outside it.
  const setMenuOpen = (open) => {
    const button = document.getElementById("settings-button");
    const panel = document.getElementById("settings-panel");
    if (!button || !panel) return;
    panel.hidden = !open;
    button.setAttribute("aria-expanded", String(open));
  };
  window.toggleSettingsMenu = () =>
    setMenuOpen(document.getElementById("settings-panel")?.hidden ?? false);
  window.closeSettingsMenu = () => setMenuOpen(false);

  const inbox = [];
  let inboxPanel;
  let inboxButton;
  window.initializeNotificationInbox = () => {
    const toggle = document.getElementById("notify-toggle");
    if (!toggle || inboxButton) return;
    const container = document.createElement("div");
    container.className = "notification-inbox";
    inboxButton = document.createElement("button");
    inboxButton.type = "button";
    inboxButton.textContent = "Inbox (0)";
    inboxButton.setAttribute("aria-expanded", "false");
    inboxButton.setAttribute("aria-controls", "notification-inbox-panel");
    inboxPanel = document.createElement("section");
    inboxPanel.id = "notification-inbox-panel";
    inboxPanel.hidden = true;
    inboxPanel.setAttribute("aria-label", "Notifications");
    const clear = document.createElement("button");
    clear.type = "button";
    clear.textContent = "Clear notifications";
    const list = document.createElement("div");
    list.className = "notification-inbox-list";
    const announcement = document.createElement("span");
    announcement.className = "notification-announcement";
    announcement.setAttribute("role", "status");
    container.append(inboxButton, inboxPanel, announcement);
    inboxPanel.append(clear, list);
    toggle.before(container);
    const render = () => {
      const unread = inbox.filter((item) => !item.read).length;
      inboxButton.textContent = "Inbox (" + unread + ")";
      inboxButton.classList.toggle("notifications-on", unread > 0);
      list.replaceChildren();
      if (!inbox.length) list.textContent = "No notifications yet.";
      for (const item of inbox) {
        const entry = document.createElement("article");
        const title = document.createElement(item.url ? "a" : "strong");
        title.textContent = item.title;
        if (item.url) title.href = item.url;
        const body = document.createElement("p");
        body.textContent = item.body;
        entry.append(title, body);
        list.append(entry);
      }
    };
    inboxButton.onclick = () => {
      inboxPanel.hidden = !inboxPanel.hidden;
      inboxButton.setAttribute("aria-expanded", String(!inboxPanel.hidden));
      if (!inboxPanel.hidden) inbox.forEach((item) => (item.read = true));
      render();
    };
    clear.onclick = () => {
      inbox.length = 0;
      render();
    };
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && !inboxPanel.hidden) {
        inboxPanel.hidden = true;
        inboxButton.setAttribute("aria-expanded", "false");
        inboxButton.focus();
      }
    });
    window.addInboxNotification = (title, body, url) => {
      inbox.unshift({ title, body, url, read: !inboxPanel.hidden });
      inbox.length = Math.min(inbox.length, 50);
      announcement.textContent = title;
      render();
    };
    render();
  };

  // Below 760px the sidebar is a drawer over the page.
  const setDrawerOpen = (open) => {
    document.body.classList.toggle("drawer-open", open);
    document
      .querySelectorAll(".drawer-toggle")
      .forEach((button) => button.setAttribute("aria-expanded", String(open)));
  };
  window.toggleDrawer = () =>
    setDrawerOpen(!document.body.classList.contains("drawer-open"));
  window.closeDrawer = () => setDrawerOpen(false);

  // The reason the notification toggle cannot be used, or "" when it can.
  // Browsers allow notifications only in a secure context, so a page opened
  // over plain HTTP from a non-loopback host cannot enable them.
  window.notificationUnavailableReason = () => {
    if (window.isSecureContext === false)
      return "Browser notifications need HTTPS or a localhost address. Forward the port with ssh -L and open http://localhost:PORT.";
    if (
      typeof Notification !== "undefined" &&
      Notification.permission === "denied"
    )
      return "Notifications are blocked for this site in the browser's settings.";
    return "";
  };

  const widthKey = "rotari-sidebar-width";
  const clampWidth = (width) => Math.max(190, Math.min(520, width));
  const saveWidth = (width) => {
    try {
      localStorage.setItem(widthKey, String(width));
    } catch {
      // The width then lasts for this page.
    }
  };
  function initSidebarResizer() {
    const sidebar = document.querySelector(".sidebar");
    const handle = document.querySelector(".sidebar-resizer");
    if (!sidebar || !handle) return;
    const applySavedWidth = () => {
      if (window.innerWidth <= 760) {
        sidebar.style.width = "";
        return;
      }
      let saved = 0;
      try {
        saved = Number(localStorage.getItem(widthKey));
      } catch {
        saved = 0;
      }
      if (Number.isFinite(saved) && saved > 0)
        sidebar.style.width = clampWidth(saved) + "px";
    };
    applySavedWidth();
    window.addEventListener("resize", applySavedWidth);
    handle.addEventListener("pointerdown", (event) => {
      event.preventDefault();
      const startX = event.clientX;
      const startWidth = sidebar.getBoundingClientRect().width;
      handle.classList.add("dragging");
      const move = (moveEvent) => {
        const width = clampWidth(startWidth + moveEvent.clientX - startX);
        sidebar.style.width = width + "px";
        saveWidth(width);
      };
      const end = () => {
        handle.classList.remove("dragging");
        window.removeEventListener("pointermove", move);
        window.removeEventListener("pointerup", end);
      };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", end);
    });
    handle.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      event.preventDefault();
      const direction = event.key === "ArrowRight" ? 1 : -1;
      const width = clampWidth(
        sidebar.getBoundingClientRect().width + direction * 12,
      );
      sidebar.style.width = width + "px";
      saveWidth(width);
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initSidebarResizer();
    document.addEventListener("click", (event) => {
      const menu = document.querySelector(".settings-menu");
      if (menu && !menu.contains(event.target)) setMenuOpen(false);
      // Following a link in the drawer closes it.
      if (event.target.closest && event.target.closest(".sidebar a"))
        setDrawerOpen(false);
    });
    document.addEventListener("keydown", (event) => {
      if (event.key !== "Escape") return;
      setMenuOpen(false);
      setDrawerOpen(false);
    });
    const select = document.getElementById("theme-choice");
    if (!select) return;
    select.value = stored();
    select.addEventListener("change", () => {
      try {
        localStorage.setItem(key, select.value);
      } catch {
        // Storage can be unavailable; the choice then lasts for this page.
      }
      apply(select.value);
    });
    // A choice made in another tab applies here too.
    window.addEventListener("storage", (event) => {
      if (event.key !== key) return;
      select.value = stored();
      apply(select.value);
    });
  });
})();
