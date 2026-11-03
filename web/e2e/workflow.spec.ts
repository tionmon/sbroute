import { test, expect } from "@playwright/test";
test("real login, import, group, entrance, responsive layout and logout", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("管理员账号").fill("admin");
  await page.getByLabel("密码", { exact: true }).fill("local-e2e-only-12345");
  await page.getByRole("button", { name: "进入控制台" }).click();
  await expect(
    page.getByRole("heading", { name: "运行概览", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: /出口节点/ }).click();
  await page
    .getByRole("button", { name: "导入节点", exact: true })
    .first()
    .click();
  let dialog = page.getByRole("dialog");
  await dialog
    .getByRole("textbox")
    .fill("socks5://user:password@127.0.0.1:19999#Tokyo%20Lab");
  await dialog.getByRole("button", { name: "解析预览" }).click();
  await expect(dialog.getByText("Tokyo Lab", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "确认导入" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("cell", { name: /Tokyo Lab/ })).toBeVisible();
  await page.getByRole("button", { name: "节点分组", exact: true }).click();
  await page
    .getByRole("button", { name: "新建分组", exact: true })
    .first()
    .click();
  dialog = page.getByRole("dialog");
  await dialog.getByLabel("分组名称").fill("Japan Pool");
  await dialog
    .getByRole("checkbox", { name: "Tokyo Lab", exact: true })
    .check();
  await dialog.getByRole("button", { name: "保存并应用" }).click();
  await expect(dialog).not.toBeVisible();
  await page.getByRole("button", { name: "分发入口", exact: true }).click();
  await page
    .getByRole("button", { name: "新建入口", exact: true })
    .first()
    .click();
  dialog = page.getByRole("dialog");
  await dialog.getByLabel("入口名称").fill("Japan proxy");
  await dialog.getByLabel("监听端口").fill("19081");
  await dialog.getByLabel("节点组").selectOption({ label: "Japan Pool" });
  await dialog.getByLabel("调度策略").selectOption("p2c");
  await dialog.getByRole("button", { name: "保存并应用" }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole("cell", { name: /Japan proxy/ })).toBeVisible();
  await page.getByRole("button", { name: "系统设置", exact: true }).click();
  await page.getByRole("button", { name: /省内存/ }).click();
  await expect(page.getByText("4096", { exact: true })).toBeVisible();
  page.once("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "回滚", exact: true }).first().click();
  await expect(page.getByText("8192", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "运行概览", exact: true }).click();
  await expect(page.getByText("Japan Pool", { exact: true })).toBeVisible();
  await expect(page.locator(".toast")).not.toBeVisible({ timeout: 6000 });
  await page.screenshot({
    path: "../bin/dashboard-desktop.png",
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByRole("heading", { name: "运行概览", exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
  await page.screenshot({
    path: "../bin/dashboard-mobile.png",
    fullPage: true,
  });
  expect(errors).toEqual([]);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole("button", { name: "退出登录", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "登录控制台", exact: true }),
  ).toBeVisible();
});
