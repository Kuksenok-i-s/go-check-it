package a

type Widget struct{}

func (w *Widget) Name() string { return "" }

func (widget *Widget) Resize() {} // want `receiver "widget" for Widget is inconsistent with "w" used elsewhere on the same type`

type Gadget struct{}

func (g *Gadget) Name() string { return "" }

func (g *Gadget) Resize() {}
