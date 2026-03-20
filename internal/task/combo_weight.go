package task

import (
	"awesomeProject/internal/config"
	"awesomeProject/internal/model"
	"awesomeProject/internal/storage"
	"awesomeProject/pkg/utils"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultComboWeightInterval          = 30 * time.Minute
	defaultComboWeightWindow            = 6 * time.Hour
	defaultComboWeightMinErrorsToAdjust = int64(5)
	defaultComboWeightLR                = 0.2
	defaultComboWeightMinWeight         = 0.05
	defaultComboWeightNormalize         = true
	defaultComboWeightMaxStep           = 0.15
	// 严重错误（429/404/403/400）惩罚倍率默认 3.0，其他错误默认 0.3
	defaultSevereErrorWeight = 3.0
	defaultMildErrorWeight   = 0.3
)

type modelErrCount struct {
	ModelID    string
	Cnt        int64
	StatusCode int
}

// severeStatusCodes 定义惩罚倍率高的错误状态码（说明 key/模型本身有问题，不是临时故障）
var severeStatusCodes = map[int]struct{}{
	400: {},
	403: {},
	404: {},
	429: {},
}

func startComboWeightAdjust(cfg *config.Config) {
	enabled := false // 默认关闭，避免意外改变路由
	interval := defaultComboWeightInterval
	window := defaultComboWeightWindow
	minErrors := defaultComboWeightMinErrorsToAdjust
	lr := defaultComboWeightLR
	minWeight := defaultComboWeightMinWeight
	normalize := defaultComboWeightNormalize
	maxStep := defaultComboWeightMaxStep
	severeErrorWeight := defaultSevereErrorWeight
	mildErrorWeight := defaultMildErrorWeight

	if cfg != nil {
		enabled = boolOrDefault(cfg.Tasks.ComboWeight.Enabled, false)
		interval = parseDurationOrDefault(cfg.Tasks.ComboWeight.Interval, defaultComboWeightInterval)
		window = parseDurationOrDefault(cfg.Tasks.ComboWeight.Window, defaultComboWeightWindow)
		if cfg.Tasks.ComboWeight.MinErrorsToAdjust > 0 {
			minErrors = cfg.Tasks.ComboWeight.MinErrorsToAdjust
		}
		if cfg.Tasks.ComboWeight.LR > 0 {
			lr = cfg.Tasks.ComboWeight.LR
		}
		if cfg.Tasks.ComboWeight.MinWeight > 0 {
			minWeight = cfg.Tasks.ComboWeight.MinWeight
		}
		normalize = boolOrDefault(cfg.Tasks.ComboWeight.Normalize, defaultComboWeightNormalize)
		if cfg.Tasks.ComboWeight.MaxStep > 0 {
			maxStep = cfg.Tasks.ComboWeight.MaxStep
		}
		if cfg.Tasks.ComboWeight.SevereErrorWeight > 0 {
			severeErrorWeight = cfg.Tasks.ComboWeight.SevereErrorWeight
		}
		if cfg.Tasks.ComboWeight.MildErrorWeight > 0 {
			mildErrorWeight = cfg.Tasks.ComboWeight.MildErrorWeight
		}
	}

	if !enabled {
		utils.Logger.Printf("[ComboWeightTask] combo weight adjust task disabled")
		return
	}
	if window <= 0 {
		window = defaultComboWeightWindow
	}
	if lr <= 0 || lr > 1 {
		utils.Logger.Printf("[ComboWeightTask] invalid lr=%v, fallback to %v", lr, defaultComboWeightLR)
		lr = defaultComboWeightLR
	}
	if minWeight <= 0 || minWeight >= 1 {
		utils.Logger.Printf("[ComboWeightTask] invalid min_weight=%v, fallback to %v", minWeight, defaultComboWeightMinWeight)
		minWeight = defaultComboWeightMinWeight
	}

	runOnce := func() {
		if err := AdjustComboWeightsByGlobalErrorLogs(window, minErrors, lr, minWeight, normalize, maxStep, severeErrorWeight, mildErrorWeight); err != nil {
			utils.Logger.Printf("[ComboWeightTask] adjust failed: %v", err)
		}
	}

	runOnce()
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			runOnce()
		}
	}()
	utils.Logger.Printf("[ComboWeightTask] started (interval=%s, window=%s, minErrors=%d)", interval, window, minErrors)
}

// AdjustComboWeightsByGlobalErrorLogs 按全局 ErrorLog(model_id) 的错误数调整每个 combo 的 item 权重。
// 规则：错误越少，权重越高，使用 score=1/(err+1) 并归一化。
// 严重错误（429/404/403/400）按 severeErrorWeight 倍计，其他错误按 mildErrorWeight 倍计。
func AdjustComboWeightsByGlobalErrorLogs(window time.Duration, minErrorsToAdjust int64, lr, minWeight float64, normalize bool, maxStep float64, severeErrorWeight, mildErrorWeight float64) error {
	since := time.Now().Add(-window)

	errMap, totalErr, err := loadGlobalErrorCountsSince(since, severeErrorWeight, mildErrorWeight)
	if err != nil {
		return err
	}
	if totalErr < float64(minErrorsToAdjust) {
		utils.Logger.Printf("[ComboWeightTask] skip: total errors=%.2f < minErrorsToAdjust=%d (since %s)", totalErr, minErrorsToAdjust, since.Format(time.RFC3339))
		return nil
	}

	var combos []model.Combo
	if err := storage.DB.Preload("Items").Find(&combos).Error; err != nil {
		return err
	}

	adjustedCombos := 0
	adjustedItems := 0

	for _, cb := range combos {
		if !cb.Enabled || len(cb.Items) == 0 {
			continue
		}

		items := make([]model.ComboItem, 0, len(cb.Items))
		autoUpdatableCount := 0
		for _, it := range cb.Items {
			if strings.TrimSpace(it.ModelID) == "" {
				continue
			}
			// 跳过不参与自动权重更新的项
			if it.AutoWeightUpdate != nil && !*it.AutoWeightUpdate {
				continue
			}
			items = append(items, it)
			autoUpdatableCount++
		}
		// 如果所有子模型都不参与自动更新，则跳过该 combo
		if autoUpdatableCount == 0 {
			continue
		}

		newWeights := computeNewWeights(items, errMap, lr, minWeight, normalize, maxStep)
		if len(newWeights) == 0 {
			continue
		}

		changed, err := applyComboItemWeights(cb.ID, newWeights)
		if err != nil {
			utils.Logger.Printf("[ComboWeightTask] apply weights failed combo=%s: %v", cb.ID, err)
			continue
		}
		if changed > 0 {
			adjustedCombos++
			adjustedItems += changed
		}
	}

	utils.Logger.Printf("[ComboWeightTask] adjusted combos=%d items=%d (since %s, totalWeightedErrors=%.2f)", adjustedCombos, adjustedItems, since.Format(time.RFC3339), totalErr)
	return nil
}

func loadGlobalErrorCountsSince(since time.Time, severeErrorWeight, mildErrorWeight float64) (map[string]float64, float64, error) {
	var rows []modelErrCount
	q := storage.DB.Model(&model.ErrorLog{}).
		Select("model_id as model_id, status_code as status_code, COUNT(1) as cnt").
		Where("created_at >= ?", since).
		Group("model_id, status_code")
	if err := q.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	m := make(map[string]float64)
	var total float64
	for _, r := range rows {
		id := strings.TrimSpace(r.ModelID)
		if id == "" {
			continue
		}
		if r.Cnt < 0 {
			r.Cnt = 0
		}

		// 根据状态码决定权重倍率
		weight := mildErrorWeight
		if _, isSevere := severeStatusCodes[r.StatusCode]; isSevere {
			weight = severeErrorWeight
		}

		// 加权累计
		weightedErr := float64(r.Cnt) * weight
		m[id] += weightedErr
		total += weightedErr
	}
	return m, total, nil
}

func computeNewWeights(items []model.ComboItem, errMap map[string]float64, lr, minWeight float64, normalize bool, maxStep float64) map[uint]float64 {
	// 目标权重 = 1/(err+1)，错误越少越大
	target := make([]float64, 0, len(items))
	for _, it := range items {
		e := errMap[strings.TrimSpace(it.ModelID)]
		if e < 0 {
			e = 0
		}
		score := 1.0 / (e + 1)
		target = append(target, score)
	}
	if normalize {
		normInPlace(target)
	}
	// 最小权重钳制（可选），再归一化
	if minWeight > 0 {
		for i := range target {
			if target[i] < minWeight {
				target[i] = minWeight
			}
		}
		if normalize {
			normInPlace(target)
		}
	}

	out := make(map[uint]float64, len(items))
	for i, it := range items {
		oldW := it.Weight
		if oldW < 0 {
			oldW = 0
		}
		newW := oldW*(1-lr) + target[i]*lr
		if maxStep > 0 {
			delta := newW - oldW
			if math.Abs(delta) > maxStep {
				newW = oldW + math.Copysign(maxStep, delta)
			}
		}
		if minWeight > 0 && newW < minWeight {
			newW = minWeight
		}
		out[it.ID] = newW
	}
	if normalize {
		// 归一化写回值，避免平滑后和不为 1
		ids := make([]uint, 0, len(out))
		ws := make([]float64, 0, len(out))
		for id, w := range out {
			ids = append(ids, id)
			ws = append(ws, w)
		}
		normInPlace(ws)
		for i, id := range ids {
			out[id] = ws[i]
		}
	}
	return out
}

func normInPlace(ws []float64) {
	sum := 0.0
	for _, w := range ws {
		if w > 0 {
			sum += w
		}
	}
	if sum <= 0 {
		// 回退：平均分配
		if len(ws) == 0 {
			return
		}
		avg := 1.0 / float64(len(ws))
		for i := range ws {
			ws[i] = avg
		}
		return
	}
	for i := range ws {
		if ws[i] < 0 {
			ws[i] = 0
		}
		ws[i] = ws[i] / sum
	}
}

func applyComboItemWeights(comboID string, newWeights map[uint]float64) (int, error) {
	if strings.TrimSpace(comboID) == "" || len(newWeights) == 0 {
		return 0, nil
	}

	// 为了输出更稳定，按 id 排序
	ids := make([]uint, 0, len(newWeights))
	for id := range newWeights {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	changed := 0
	err := storage.DB.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			w := newWeights[id]
			if w < 0 {
				w = 0
			}
			res := tx.Model(&model.ComboItem{}).Where("id = ? AND combo_id = ?", id, comboID).Update("weight", w)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				changed++
			}
		}
		return nil
	})
	return changed, err
}
