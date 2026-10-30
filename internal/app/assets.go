package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/12312bg/mdbmc/internal/auth"
	"github.com/12312bg/mdbmc/internal/bmc"
	"github.com/12312bg/mdbmc/internal/store"
)

// FileDialogs shows the system's file dialogs. The chosen path is "" when
// the user cancels.
type FileDialogs interface {
	SaveFile(title, defaultName, filterName, pattern string) (string, error)
	OpenFile(title, filterName, pattern string) (string, error)
}

// SetFileDialogs supplies the file dialogs (not available in a browser).
func (a *App) SetFileDialogs(d FileDialogs) { a.dialogs = d }

var errNoDialogs = errors.New("当前运行方式不支持选择文件")

func (a *App) saveDialog(title, defaultName, filterName, pattern string) (string, error) {
	if a.dialogs == nil {
		return "", errNoDialogs
	}
	return a.dialogs.SaveFile(title, defaultName, filterName, pattern)
}

func (a *App) openDialog(title, filterName, pattern string) (string, error) {
	if a.dialogs == nil {
		return "", errNoDialogs
	}
	return a.dialogs.OpenFile(title, filterName, pattern)
}

const xlsxPattern = "*.xlsx"

// ExportAssets saves the device list, with the hardware details collected
// so far, as an Excel workbook. Passwords are never exported. It returns
// the file written, or "" when the user cancelled.
func (s *DeviceService) ExportAssets() (string, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceView)
	if err != nil {
		return "", err
	}
	path, err := s.a.saveDialog("导出设备资产", "设备资产-"+time.Now().Format("20060102")+".xlsx", "Excel 工作簿", xlsxPattern)
	if err != nil || path == "" {
		return "", err
	}
	n, err := s.a.exportAssets(s.a.bg(), path)
	s.a.audit(sess, "device.export", filepath.Base(path), err, fmt.Sprintf("%d 台设备", n))
	if err != nil {
		return "", err
	}
	return path, nil
}

// cell builders for the workbook
type sheet struct {
	name   string
	header []string
	widths []float64
	rows   [][]any
}

func (a *App) exportAssets(ctx context.Context, path string) (int, error) {
	devs, err := a.store.ListDevices(ctx)
	if err != nil {
		return 0, err
	}
	groups, creds, err := a.lookups(ctx)
	if err != nil {
		return 0, err
	}
	yesNo := map[bool]string{true: "在线", false: "离线"}
	power := map[string]string{"on": "已开机", "off": "已关机"}
	health := map[string]string{"ok": "正常", "warning": "警告", "critical": "严重"}
	protocol := map[string]string{"auto": "自动", "ipmi": "仅 IPMI", "redfish": "仅 Redfish"}
	ts := func(t int64) string {
		if t == 0 {
			return ""
		}
		return time.Unix(t, 0).Format("2006-01-02 15:04:05")
	}
	or := func(m map[string]string, k string) string {
		if v, ok := m[k]; ok {
			return v
		}
		return "未知"
	}

	main := sheet{
		name: "设备",
		header: []string{"名称", "地址", "品牌", "型号", "序列号", "分组", "BMC 固件", "BIOS 版本", "电源", "健康", "连接",
			"管理协议", "IPMI 端口", "HTTPS 端口", "凭据模板", "BMC 用户名", "备注", "最后检查", "添加时间"},
		widths: []float64{18, 16, 12, 22, 18, 14, 14, 14, 8, 8, 8, 10, 10, 11, 14, 14, 24, 19, 19},
	}
	cpu := sheet{name: "处理器", header: []string{"设备", "地址", "插槽", "型号", "厂商", "核心", "线程", "最高频率 (MHz)"},
		widths: []float64{18, 16, 12, 40, 14, 8, 8, 14}}
	mem := sheet{name: "内存", header: []string{"设备", "地址", "插槽", "容量 (GB)", "类型", "频率 (MHz)", "厂商", "料号", "序列号"},
		widths: []float64{18, 16, 16, 10, 10, 11, 14, 22, 16}}
	disk := sheet{name: "磁盘", header: []string{"设备", "地址", "名称", "型号", "容量 (GB)", "介质", "接口", "固件", "序列号"},
		widths: []float64{18, 16, 20, 28, 10, 8, 8, 12, 22}}
	psu := sheet{name: "电源", header: []string{"设备", "地址", "名称", "型号", "额定功率 (W)", "固件", "序列号"},
		widths: []float64{18, 16, 16, 24, 12, 12, 22}}

	for _, d := range devs {
		cred, user := "", d.Username
		if d.CredentialID != 0 {
			cred, user = creds[d.CredentialID], ""
		}
		online := yesNo[d.Reachable]
		if d.CheckedAt == 0 {
			online = "未检查"
		}
		main.rows = append(main.rows, []any{d.Name, d.Host, bmc.VendorName(d.Vendor), d.Model, d.Serial, groups[d.GroupID],
			d.BMCFirmware, d.BIOSVersion, or(power, d.PowerState), or(health, d.Health), online, protocol[d.Protocol],
			d.IPMIPort, d.HTTPSPort, cred, user, d.Note, ts(d.CheckedAt), ts(d.CreatedAt)})

		data, _, err := a.store.Inventory(ctx, d.ID)
		if err != nil {
			continue // not collected yet
		}
		var inv bmc.Inventory
		if json.Unmarshal([]byte(data), &inv) != nil {
			continue
		}
		for _, p := range inv.Processors {
			cpu.rows = append(cpu.rows, []any{d.Name, d.Host, p.Socket, p.Model, p.Manufacturer, p.Cores, p.Threads, p.MaxSpeedMHz})
		}
		for _, m := range inv.Memory {
			mem.rows = append(mem.rows, []any{d.Name, d.Host, m.Slot, roundGB(float64(m.CapacityMiB) / 1024), m.Type, m.SpeedMHz, m.Manufacturer, m.PartNumber, m.Serial})
		}
		for _, dr := range inv.Drives {
			disk.rows = append(disk.rows, []any{d.Name, d.Host, dr.Name, dr.Model, roundGB(float64(dr.CapacityBytes) / 1e9), dr.MediaType, dr.Protocol, dr.Firmware, dr.Serial})
		}
		for _, p := range inv.PowerSupplies {
			psu.rows = append(psu.rows, []any{d.Name, d.Host, p.Name, p.Model, p.CapacityWatts, p.Firmware, p.Serial})
		}
	}

	f := excelize.NewFile()
	defer f.Close()
	sheets := []sheet{main}
	for _, sh := range []sheet{cpu, mem, disk, psu} {
		if len(sh.rows) > 0 {
			sheets = append(sheets, sh)
		}
	}
	for i, sh := range sheets {
		if err := writeSheet(f, i == 0, sh); err != nil {
			return 0, err
		}
	}
	return len(devs), f.SaveAs(path)
}

func roundGB(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

func (a *App) lookups(ctx context.Context) (groups, creds map[int64]string, err error) {
	gs, err := a.store.ListGroups(ctx)
	if err != nil {
		return nil, nil, err
	}
	cs, err := a.store.ListCredentials(ctx)
	if err != nil {
		return nil, nil, err
	}
	groups, creds = map[int64]string{}, map[int64]string{}
	for _, g := range gs {
		groups[g.ID] = g.Name
	}
	for _, c := range cs {
		creds[c.ID] = c.Name
	}
	return groups, creds, nil
}

// writeSheet adds a sheet with a bold, frozen and filterable header row.
func writeSheet(f *excelize.File, first bool, sh sheet) error {
	if first {
		if err := f.SetSheetName(f.GetSheetName(0), sh.name); err != nil {
			return err
		}
	} else if _, err := f.NewSheet(sh.name); err != nil {
		return err
	}
	head, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F2937"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E8F1FC"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border:    []excelize.Border{{Type: "bottom", Color: "B6C6DA", Style: 1}},
	})
	if err != nil {
		return err
	}
	header := make([]any, len(sh.header))
	for i, h := range sh.header {
		header[i] = h
	}
	if err := f.SetSheetRow(sh.name, "A1", &header); err != nil {
		return err
	}
	last, _ := excelize.CoordinatesToCellName(len(sh.header), 1)
	if err := f.SetCellStyle(sh.name, "A1", last, head); err != nil {
		return err
	}
	_ = f.SetRowHeight(sh.name, 1, 22)
	for i, w := range sh.widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sh.name, col, col, w)
	}
	for i, r := range sh.rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		row := r
		if err := f.SetSheetRow(sh.name, cell, &row); err != nil {
			return err
		}
	}
	if err := f.SetPanes(sh.name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	return f.AutoFilter(sh.name, "A1:"+last, nil)
}

// importSheet is the sheet of the import template.
const importSheet = "设备导入"

// importColumns are the template's columns; aliases are accepted too.
var importColumns = []struct {
	key     string
	title   string
	aliases []string
	width   float64
	help    string
}{
	{"name", "名称", []string{"设备名称", "名字"}, 18, "可选，留空时使用地址"},
	{"host", "地址", []string{"bmc 地址", "ip", "ip 地址", "bmc ip", "主机"}, 16, "必填，BMC 的 IP 地址或主机名，不含端口"},
	{"ipmiPort", "IPMI 端口", nil, 10, "可选，默认 623"},
	{"httpsPort", "HTTPS 端口", nil, 11, "可选，默认 443"},
	{"protocol", "管理协议", []string{"协议"}, 11, "可选：自动、仅 IPMI、仅 Redfish，默认自动"},
	{"group", "分组", nil, 14, "可选，填分组名称；不存在时自动创建"},
	{"credential", "凭据模板", []string{"凭据"}, 14, "填写已有凭据模板的名称；填了它就不必填用户名和密码"},
	{"username", "BMC 用户名", []string{"用户名"}, 14, "不使用凭据模板时必填"},
	{"password", "BMC 密码", []string{"密码"}, 14, "不使用凭据模板时填写；导入后加密保存，请删除本文件"},
	{"note", "备注", nil, 24, "可选"},
}

const maxImportRows = 2000

// ImportTemplate saves an empty import workbook with instructions.
func (s *DeviceService) ImportTemplate() (string, error) {
	if _, err := s.a.auth.Require(auth.PermDeviceManage); err != nil {
		return "", err
	}
	path, err := s.a.saveDialog("保存设备导入模板", "设备导入模板.xlsx", "Excel 工作簿", xlsxPattern)
	if err != nil || path == "" {
		return "", err
	}
	if err := writeImportTemplate(path); err != nil {
		return "", err
	}
	return path, nil
}

func writeImportTemplate(path string) error {
	f := excelize.NewFile()
	defer f.Close()
	tpl := sheet{name: importSheet}
	help := sheet{name: "填写说明", header: []string{"列", "说明"}, widths: []float64{14, 60}}
	for _, c := range importColumns {
		tpl.header = append(tpl.header, c.title)
		tpl.widths = append(tpl.widths, c.width)
		help.rows = append(help.rows, []any{c.title, c.help})
	}
	help.rows = append(help.rows, []any{}, []any{"提示", "每行一台设备，最多 2000 行；地址和端口与已有设备相同的行会跳过。"})
	if err := writeSheet(f, true, tpl); err != nil {
		return err
	}
	if err := writeSheet(f, false, help); err != nil {
		return err
	}
	return f.SaveAs(path)
}

// ImportRow is one row of an import preview.
type ImportRow struct {
	Row         int    `json:"row"` // row number in the sheet
	Name        string `json:"name"`
	Host        string `json:"host"`
	IPMIPort    int    `json:"ipmiPort"`
	HTTPSPort   int    `json:"httpsPort"`
	Protocol    string `json:"protocol"`
	Group       string `json:"group"`
	Credential  string `json:"credential"`
	Username    string `json:"username"`
	HasPassword bool   `json:"hasPassword"`
	Note        string `json:"note"`
	// Status is ok (will be added), skip (already managed) or error.
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ImportPreview is what a file would import.
type ImportPreview struct {
	Token     string      `json:"token"`
	File      string      `json:"file"`
	Rows      []ImportRow `json:"rows"`
	OK        int         `json:"ok"`
	Skip      int         `json:"skip"`
	Errors    int         `json:"errors"`
	NewGroups []string    `json:"newGroups"`
}

// ImportResult reports an import.
type ImportResult struct {
	Added  int      `json:"added"`
	Failed []string `json:"failed"`
}

// pendingImport is a previewed file waiting to be imported; it stays in
// the backend so the passwords in it never go to the window.
type pendingImport struct {
	token   string
	userID  int64
	file    string
	created time.Time
	items   []importItem
}

type importItem struct {
	row   ImportRow
	input DeviceInput
}

var importMu sync.Mutex

// PreviewImport asks for a workbook and checks each row; Import then adds
// the valid ones. It returns nil when the user cancelled.
func (s *DeviceService) PreviewImport() (*ImportPreview, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	path, err := s.a.openDialog("选择设备导入文件", "Excel 工作簿", xlsxPattern)
	if err != nil || path == "" {
		return nil, err
	}
	return s.a.previewImport(s.a.bg(), sess, path)
}

func (a *App) previewImport(ctx context.Context, sess *auth.Session, path string) (*ImportPreview, error) {
	items, err := readImportFile(path)
	if err != nil {
		return nil, err
	}
	devs, err := a.store.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	gs, err := a.store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := a.store.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := func(host string, ipmi, https int) string {
		return strings.ToLower(host) + "|" + strconv.Itoa(ipmi) + "|" + strconv.Itoa(https)
	}
	existing := map[string]bool{}
	for _, d := range devs {
		existing[endpoint(d.Host, d.IPMIPort, d.HTTPSPort)] = true
	}
	groupID := map[string]int64{}
	for _, g := range gs {
		groupID[strings.ToLower(g.Name)] = g.ID
	}
	credID := map[string]int64{}
	for _, c := range cs {
		credID[strings.ToLower(c.Name)] = c.ID
	}

	p := &ImportPreview{File: filepath.Base(path), Rows: make([]ImportRow, 0, len(items)), NewGroups: []string{}}
	newGroups := map[string]bool{}
	seen := map[string]int{}
	for i := range items {
		it := &items[i]
		r := &it.row
		if r.Status == "" && r.Credential != "" {
			if id, ok := credID[strings.ToLower(r.Credential)]; ok {
				it.input.CredentialID = id
				it.input.Username, it.input.Password = "", ""
			} else {
				r.Status, r.Message = "error", "凭据模板“"+r.Credential+"”不存在"
			}
		}
		if r.Status == "" {
			if err := normalizeDeviceInput(&it.input); err != nil {
				r.Status, r.Message = "error", err.Error()
			}
		}
		if r.Status == "" {
			in := it.input
			r.Name, r.Host, r.IPMIPort, r.HTTPSPort, r.Protocol = in.Name, in.Host, in.IPMIPort, in.HTTPSPort, in.Protocol
			key := endpoint(in.Host, in.IPMIPort, in.HTTPSPort)
			switch {
			case existing[key]:
				r.Status, r.Message = "skip", "该地址和端口的设备已存在，跳过"
			case seen[key] != 0:
				r.Status, r.Message = "error", fmt.Sprintf("与第 %d 行的地址和端口重复", seen[key])
			default:
				seen[key] = r.Row
				r.Status = "ok"
				if g := strings.TrimSpace(r.Group); g != "" {
					if _, ok := groupID[strings.ToLower(g)]; !ok && !newGroups[strings.ToLower(g)] {
						newGroups[strings.ToLower(g)] = true
						p.NewGroups = append(p.NewGroups, g)
					}
				}
			}
		}
		switch r.Status {
		case "ok":
			p.OK++
		case "skip":
			p.Skip++
		default:
			p.Errors++
		}
		p.Rows = append(p.Rows, *r)
	}

	token := make([]byte, 16)
	_, _ = rand.Read(token)
	p.Token = hex.EncodeToString(token)
	importMu.Lock()
	a.pending = &pendingImport{token: p.Token, userID: sess.UserID, file: p.File, created: time.Now(), items: items}
	importMu.Unlock()
	return p, nil
}

// readImportFile reads the device rows of an import workbook.
func readImportFile(path string) ([]importItem, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件：%w", err)
	}
	defer f.Close()
	name := importSheet
	if idx, _ := f.GetSheetIndex(name); idx < 0 {
		name = f.GetSheetName(0)
	}
	rows, err := f.GetRows(name)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("文件中没有内容")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		h = strings.ToLower(strings.Join(strings.Fields(h), " "))
		for _, c := range importColumns {
			if h == strings.ToLower(c.title) || containsFold(c.aliases, h) {
				col[c.key] = i
			}
		}
	}
	if _, ok := col["host"]; !ok {
		return nil, errors.New("找不到“地址”列，请使用导入模板")
	}
	get := func(row []string, key string) string {
		if i, ok := col[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var items []importItem
	for n, row := range rows[1:] {
		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		if len(items) == maxImportRows {
			return nil, fmt.Errorf("一次最多导入 %d 台设备", maxImportRows)
		}
		r := ImportRow{Row: n + 2, Name: get(row, "name"), Host: get(row, "host"), Group: get(row, "group"),
			Credential: get(row, "credential"), Username: get(row, "username"), Note: get(row, "note")}
		pw := get(row, "password")
		r.HasPassword = pw != ""
		in := DeviceInput{Name: r.Name, Host: r.Host, Username: r.Username, Password: pw, Note: r.Note}
		var bad []string
		for key, p := range map[string]*int{"ipmiPort": &in.IPMIPort, "httpsPort": &in.HTTPSPort} {
			if v := get(row, key); v != "" {
				n, err := strconv.ParseFloat(v, 64)
				if err != nil || n != float64(int(n)) {
					bad = append(bad, "端口“"+v+"”不是整数")
				}
				*p = int(n)
			}
		}
		if in.Protocol, err = parseProtocol(get(row, "protocol")); err != nil {
			bad = append(bad, err.Error())
		}
		r.IPMIPort, r.HTTPSPort, r.Protocol = in.IPMIPort, in.HTTPSPort, in.Protocol
		if len(bad) > 0 {
			r.Status, r.Message = "error", strings.Join(bad, "；")
		}
		items = append(items, importItem{row: r, input: in})
	}
	if len(items) == 0 {
		return nil, errors.New("文件中没有设备行")
	}
	return items, nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func parseProtocol(s string) (string, error) {
	v := strings.ToLower(strings.NewReplacer("仅", "", " ", "", "只", "").Replace(s))
	switch v {
	case "", "自动", "auto":
		return string(bmc.ProtocolAuto), nil
	case "ipmi":
		return string(bmc.ProtocolIPMI), nil
	case "redfish":
		return string(bmc.ProtocolRedfish), nil
	}
	return "", fmt.Errorf("管理协议“%s”无效，应为自动、仅 IPMI 或仅 Redfish", s)
}

// Import adds the valid rows of the previewed file and identifies the new
// devices in the background.
func (s *DeviceService) Import(token string) (*ImportResult, error) {
	sess, err := s.a.auth.Require(auth.PermDeviceManage)
	if err != nil {
		return nil, err
	}
	importMu.Lock()
	p := s.a.pending
	if p == nil || p.token != token || p.userID != sess.UserID || time.Since(p.created) > 30*time.Minute {
		importMu.Unlock()
		return nil, errors.New("导入预览已失效，请重新选择文件")
	}
	s.a.pending = nil
	importMu.Unlock()

	ctx := s.a.bg()
	gs, err := s.a.store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	groupID := map[string]int64{}
	for _, g := range gs {
		groupID[strings.ToLower(g.Name)] = g.ID
	}
	res := &ImportResult{Failed: []string{}}
	var ids []int64
	for _, it := range p.items {
		if it.row.Status != "ok" {
			continue
		}
		in := it.input
		if g := strings.TrimSpace(it.row.Group); g != "" {
			id, ok := groupID[strings.ToLower(g)]
			if !ok {
				if id, err = s.a.store.CreateGroup(ctx, g); err != nil {
					res.Failed = append(res.Failed, fmt.Sprintf("第 %d 行：%v", it.row.Row, err))
					continue
				}
				groupID[strings.ToLower(g)] = id
			}
			in.GroupID = id
		}
		d := &store.Device{
			Name: in.Name, Host: in.Host, IPMIPort: in.IPMIPort, HTTPSPort: in.HTTPSPort, Protocol: in.Protocol,
			GroupID: in.GroupID, CredentialID: in.CredentialID, Note: in.Note,
		}
		if in.CredentialID == 0 {
			d.Username = in.Username
			d.PasswordEnc = s.a.sealDevicePassword(sess, in.Password)
		}
		if err := s.a.store.CreateDevice(ctx, d); err != nil {
			res.Failed = append(res.Failed, fmt.Sprintf("第 %d 行（%s）：%v", it.row.Row, in.Host, err))
			continue
		}
		ids = append(ids, d.ID)
	}
	res.Added = len(ids)
	var failErr error
	if len(res.Failed) > 0 {
		failErr = fmt.Errorf("%d 行失败", len(res.Failed))
	}
	s.a.audit(sess, "device.import", p.file, failErr, fmt.Sprintf("导入 %d 台设备", res.Added))
	if len(ids) > 0 {
		go s.a.identify(sess, ids)
	}
	return res, nil
}
