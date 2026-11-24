package items

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"app/internal/database"
	"app/internal/httpx"
	"app/internal/media"
	"app/internal/seo"
	"app/internal/validate"
)

type handlers struct {
	repo  *Repo
	media *media.Service
}

// Routes registers the module. requireAuth comes from internal/web.
func Routes(app fiber.Router, repo *Repo, m *media.Service, requireAuth fiber.Handler) {
	h := &handlers{repo: repo, media: m}
	app.Get("/i/:id", h.public) // public, indexable page for published items

	g := app.Group("/items", requireAuth)
	g.Get("/", h.list)
	g.Get("/new", h.newForm)
	g.Post("/", h.create)
	g.Get("/:id/edit", h.editForm)
	g.Post("/:id", h.update)
	g.Post("/:id/delete", h.remove)
}

// list serves the full page, or only the results fragment for htmx
// (search-as-you-type, filters, sorting and pagination all hit this).
func (h *handlers) list(c fiber.Ctx) error {
	p := Spec.Parse(httpx.Query(c))
	page, err := h.repo.List(c.Context(), httpx.UserID(c), p)
	if err != nil {
		return err
	}
	if httpx.IsPartial(c) {
		return httpx.Render(c, Results(page))
	}
	httpx.SetMeta(c, seo.Meta{Title: "Items", NoIndex: true})
	return httpx.Render(c, ListPage(page))
}

type form struct {
	Title, Body, Status, CoverID string
}

func readForm(c fiber.Ctx) (form, *validate.V) {
	f := form{
		Title:   strings.TrimSpace(c.FormValue("title")),
		Body:    strings.TrimSpace(c.FormValue("body")),
		Status:  c.FormValue("status", "draft"),
		CoverID: c.FormValue("cover_id"),
	}
	v := validate.New().
		Required("title", f.Title).MaxLen("title", f.Title, 200).
		MaxLen("body", f.Body, 20000).
		OneOf("status", f.Status, Statuses...)
	return f, v
}

func (h *handlers) newForm(c fiber.Ctx) error {
	httpx.SetMeta(c, seo.Meta{Title: "New item", NoIndex: true})
	return httpx.Render(c, FormPage("", form{Status: "draft"}, validate.Errors{}))
}

func (h *handlers) create(c fiber.Ctx) error {
	f, v := readForm(c)
	cover := h.ownCover(c, f.CoverID, v)
	if !v.OK() {
		c.Status(fiber.StatusUnprocessableEntity)
		return httpx.Render(c, FormPage("", f, v.Errors))
	}
	it := &Item{OwnerID: httpx.UserID(c), Title: f.Title, Body: f.Body, Status: f.Status, CoverID: cover}
	if err := h.repo.Create(c.Context(), it); err != nil {
		return err
	}
	httpx.Toast(c, "success", "Item created.")
	return httpx.Redirect(c, "/items")
}

func (h *handlers) editForm(c fiber.Ctx) error {
	it, err := h.own(c)
	if err != nil {
		return err
	}
	f := form{Title: it.Title, Body: it.Body, Status: it.Status}
	if it.CoverID != nil {
		f.CoverID = *it.CoverID
	}
	httpx.SetMeta(c, seo.Meta{Title: "Edit " + it.Title, NoIndex: true})
	return httpx.Render(c, FormPage(it.ID, f, validate.Errors{}))
}

func (h *handlers) update(c fiber.Ctx) error {
	it, err := h.own(c)
	if err != nil {
		return err
	}
	f, v := readForm(c)
	cover := h.ownCover(c, f.CoverID, v)
	if !v.OK() {
		c.Status(fiber.StatusUnprocessableEntity)
		return httpx.Render(c, FormPage(it.ID, f, v.Errors))
	}
	it.Title, it.Body, it.Status, it.CoverID = f.Title, f.Body, f.Status, cover
	if err := h.repo.Update(c.Context(), it); err != nil {
		return err
	}
	httpx.Toast(c, "success", "Saved.")
	return httpx.Redirect(c, "/items")
}

func (h *handlers) remove(c fiber.Ctx) error {
	if !validate.IsUUID(c.Params("id")) {
		return fiber.ErrNotFound
	}
	if err := h.repo.Delete(c.Context(), c.Params("id"), httpx.UserID(c)); err != nil {
		if database.IsNotFound(err) {
			return fiber.ErrNotFound
		}
		return err
	}
	httpx.Toast(c, "success", "Deleted.")
	if httpx.IsHTMX(c) {
		return c.SendString("") // row removed in place
	}
	return httpx.Redirect(c, "/items")
}

func (h *handlers) public(c fiber.Ctx) error {
	it, err := h.repo.Get(c.Context(), c.Params("id"))
	if err != nil || it.Status != "published" {
		return fiber.ErrNotFound
	}
	var cover *media.Media
	if it.CoverID != nil {
		if m, err := h.media.Get(c.Context(), *it.CoverID); err == nil && m.Visibility == "public" && m.Status == "ready" {
			cover = m
		}
	}
	vd := httpx.VD(c.Context())
	url := vd.BaseURL + "/i/" + it.ID
	img := ""
	if cover != nil {
		img = seo.Abs(vd.BaseURL, cover.URL(cover.Best()))
	}
	httpx.SetMeta(c, seo.Meta{
		Title: it.Title, Description: excerpt(it.Body, 160), Path: "/i/" + it.ID, Type: "article", Image: img,
		Published: it.CreatedAt, Modified: it.UpdatedAt,
		JSONLD: []any{seo.Article(it.Title, excerpt(it.Body, 160), url, img, it.CreatedAt, it.UpdatedAt, vd.OrgName)},
	})
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	return httpx.Render(c, PublicPage(it, cover))
}

// own loads an item and 404s unless the current user owns it.
func (h *handlers) own(c fiber.Ctx) (*Item, error) {
	it, err := h.repo.Get(c.Context(), c.Params("id"))
	if err != nil || it.OwnerID != httpx.UserID(c) {
		return nil, fiber.ErrNotFound
	}
	return it, nil
}

// ownCover validates that a referenced upload belongs to the user.
func (h *handlers) ownCover(c fiber.Ctx, id string, v *validate.V) *string {
	if id == "" {
		return nil
	}
	m, err := h.media.Get(c.Context(), id)
	if err != nil || m.OwnerID == nil || *m.OwnerID != httpx.UserID(c) || m.Kind != media.Image {
		v.Add("cover_id", "Choose one of your uploaded images.")
		return nil
	}
	return &m.ID
}

func excerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
