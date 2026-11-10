# OneImg API Token 多凭据管理与权限规范

> 功能开发分支：`feat/multi-api-token-scopes-expiry-20261110`。未合并、部署前，生产环境仍保持旧接口行为。
> 适用对象：OneImg 管理员、持有 `setting:api` 的用户、ScriptCat / 外部程序开发者。

## 目标与安全基线

- 同一账户可并存多个 Token：每个有名称、备注、独立授权、到期时间、启停和撤销状态。
- 创建新 Token **不会撤销其他 Token**。最多 30 个未过期、未撤销的 Token（暂停也占名额），超额返回 HTTP 409。
- Token 明文仅创建时返回一次。数据库只存 SHA-256 摘要，不能找回旧 Token；列表仅显示末尾六位提示、名称和元数据。
- 有效期可在 5 分钟到 365 天之间选择；**不允许永不过期**，过期及已撤销的 Token 不可重新启用。
- 超管 Token 也**不能**借超管权限访问或删除其他用户的图片，不能操作系统设置或管理其他 Token。
- Token 管理接口只接受已登录的 Cookie 会话，且账户必须具有 `setting:api`；任何 Bearer Token 都不能访问管理接口。
- `start_api=false` 时所有 Bearer Token 停止生效；这不会删除 Token。

## Token 权限矩阵

| scope | 允许的操作 | 说明 |
|---|---|---|
| `image:read` | `GET /api/images`、`GET /api/images/:id` | 只可读取所属账户的图片 |
| `upload:write` | `POST /api/upload`、`POST /api/upload/images`、`POST /api/images/url` | 上传本地图片或导入公网 URL |
| `image:delete` | `DELETE /api/images/:id` | 只可删除所属账户的图片；**单独勾选高风险权限** |
| `folder:read` | `GET /api/folders` | 查看所属账户文件夹 |
| `folder:write` | `GET /api/folders`、`POST /api/folders`、`PUT /api/folders/:id`、`DELETE /api/folders/:id`、`PUT /api/images/folder` | 管理本人文件夹及图片归档 |

补充：`GET /api/uploadConfig`、`GET /api/buckets/list`、`GET /api/tags` 允许 `image:read` **或** `upload:write`；`GET /api/user/status` 对任意有效 Token 开放，用于检查连接。存储桶和文件夹的实际所有权、容量和角色限制继续由各控制器校验。

不支持的权限：`setting:*`、`storage:*`、`user:*`、`*`；API Token 不可用于后台设置、用户/存储管理、创建/编辑全局标签或 R2 直传任务接口。任何不在上述方法及路径白名单中的请求都拒绝。

## 一、凭据管理接口

管理接口统一要求：

```http
Cookie: <已登录 OneImg 后台的同源浏览器会话>
Content-Type: application/json
```

不要将 Bearer Token 当作管理凭据。Token ID 是由随机密钥的摘要衍生的管理标识，**不是 Token 明文**。

### 1.1 获取 Token 列表

`GET /api/tokens?page=1&limit=50`

成功示意：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "tokens": [
      {
        "id": "<opaque 64-character id>",
        "name": "ScriptCat 图片上传",
        "description": "浏览器图片助手",
        "token_hint": "oi_…xYz123",
        "scopes": ["image:read", "upload:write"],
        "expires_at": "2026-12-10T00:00:00Z",
        "last_used_at": null,
        "created_at": "2026-11-10T00:00:00Z",
        "updated_at": "2026-11-10T00:00:00Z",
        "disabled": false,
        "revoked": false,
        "legacy": false,
        "status": "active"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 50,
    "active_count": 1,
    "max_active": 30,
    "available_scopes": [
      {"code": "image:read", "name": "查看图片", "description": "读取自己的图片、详情、上传配置与标签"}
    ]
  }
}
```

`available_scopes` 实际返回全部可授权权限；样例仅展示一项。Token 状态 `active` / `disabled` / `expired` / `revoked`。

### 1.2 创建 Token

`POST /api/tokens`

```json
{
  "name": "ScriptCat 图片上传",
  "description": "浏览器上传，禁止删除",
  "scopes": ["image:read", "upload:write"],
  "expires_at": "2026-12-10T00:00:00Z"
}
```

响应 **HTTP 201**，JSON 内 `code:200`：

```json
{
  "code": 200,
  "message": "Token 已创建，明文仅显示本次",
  "data": {
    "token": "oi_<只显示一次的实际密钥>",
    "api_token": "oi_<相同密钥兼容字段>",
    "credential": {"id":"<opaque id>","name":"ScriptCat 图片上传","scopes":["image:read","upload:write"]}
  }
}
```

只有创建响应包含明文密钥。务必立即保存到 ScriptCat 私有设置中，不要上传、打印或提交到 GitHub。

### 1.3 编辑 Token

`PUT /api/tokens/:id`；请求体与创建相同，**需要提交完整** `name`、`description`、`scopes`、`expires_at`。编辑会立即生效，包括扩大或缩小权限和更新有效期。已经过期、已经撤销或 legacy Token 不可编辑；此时请新建 Token。

### 1.4 暂停 / 恢复 Token

`PUT /api/tokens/:id/status`

```json
{"disabled": true}
```

恢复传 `false`。暂停后该 Token 立刻返回 401；恢复后原密钥继续生效。已过期、已撤销的 Token 不能恢复。

### 1.5 永久撤销 Token

`DELETE /api/tokens/:id`

永久撤销不可撤回，相关外部程序立即失去授权；不会影响同一账户其他 Token。

### 1.6 兼容旧接口

`POST /api/settings/regenerate` 为历史兼容接口（Cookie 会话），更新后会**新增**一个默认权限、30 天到期的 Token，**不会再全局撤销旧 Token**。新脚本优先使用 `POST /api/tokens`。

旧版已经签发的 Token 保留原始有效期与授权。旧版凭据会以“历史 API Token”呈现在列表中，允许撤销，但不能直接编辑，也无法读取完整明文。

## 二、使用 Token 调用图片 API

新建的 Token 仍按既有格式请求：

```http
Authorization: Bearer oi_your_token_here
```

例如（上传一张图片）：

```bash
curl -X POST "https://img.example.com/api/upload/images" \
  -H "Authorization: Bearer oi_your_token_here" \
  -F "images[]=@sample.png"
```

只读 Token 无法上传；只上传 Token 可以读取 `uploadConfig`、桶列表和标签以选择上传目的地，但不能枚举图片图库；只有显式配置 `image:delete` 才能调用删除。ScriptCat 最小推荐 scope 为 `image:read` + `upload:write`。

## 三、错误与迁移

| 状态码 | 可能原因 |
|---|---|
| 400 | 请求字段无效、scope 未知/重复、到期时间越界 |
| 401 | Token 错误、到期、暂停、撤销，或 API 总开关关闭 |
| 403 | 登录会话缺少 `setting:api`、API scope 不足或跨账户访问 |
| 404 | Token ID 不存在，或不是当前管理用户的 Token |
| 409 | 单账户有效 Token 上限 30 个；legacy Token 不能编辑 |
| 500 | 数据库或服务异常 |

数据库使用 GORM `AutoMigrate`，在原 `api_credentials` 表追加名称、备注、提示、禁用和撤销时间等字段；旧 Token 摘要、权限和有效期不被清空。既有部署升级后应先进行数据库备份，确认已存在的 ScriptCat Token 仍可上传，再逐步替换为新命名 Token。

**脚本跨站请求提示：**多 Token 改造本身不改变 OneImg 的 `SameOriginWrites` / `Sec-Fetch-Site` 策略。ScriptCat 的 `GM_xmlhttpRequest` 仍须在真实浏览器中验证；浏览器扩展的跨站请求可能被 OneImg 的 Origin 检查拦截。不要因此全局关闭跨站请求保护。

源码位置：
- `backend/models/APICredential.go`
- `backend/utils/authsecurity/tokens.go`
- `backend/controllers/auth/APITokens.go`
- `backend/middlewares/AuthMiddleware.go`
- `backend/routes/routes.go`
- `frontend/src/components/settings/APITokenManager.vue`

相关文档：[ScriptCat 图片上传 API](oneimg-scriptcat-api.md)。
