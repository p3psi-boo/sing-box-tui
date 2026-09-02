package app

func groupRowHeight(rows []GroupRow, i, viewStart int) int {
	if i > viewStart && i < len(rows) && rows[i].IsHeader {
		return 2
	}
	return 1
}

func visibleGroups(rows []GroupRow, cursor, offset, height int) (start, end, newOffset int) {
	n := len(rows)
	if n == 0 || height < 1 {
		return 0, 0, 0
	}
	cursor = clamp(cursor, 0, n-1)
	if offset < 0 {
		offset = 0
	}
	if cursor < offset {
		offset = cursor
	}
	fillEnd := func(off int) int {
		used := 0
		i := off
		for i < n {
			h := groupRowHeight(rows, i, off)
			if used > 0 && used+h > height {
				break
			}
			used += h
			i++
			if used >= height {
				break
			}
		}
		if i == off && off < n {
			return off + 1
		}
		return i
	}
	end = fillEnd(offset)
	for cursor >= end && offset < cursor {
		offset++
		end = fillEnd(offset)
	}
	return offset, end, offset
}

func visibleWindow(n, cursor, offset, height int) (start, end, newOffset int) {
	if n <= 0 {
		return 0, 0, 0
	}
	if height < 1 {
		height = 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}
	if n <= height {
		return 0, n, 0
	}
	if offset < 0 {
		offset = 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+height {
		offset = cursor - height + 1
	}
	maxOff := n - height
	if offset > maxOff {
		offset = maxOff
	}
	if offset < 0 {
		offset = 0
	}
	return offset, offset + height, offset
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func pageStep(height int) int {
	step := height - 1
	if step < 5 {
		return 5
	}
	return step
}

func halfPageStep(height int) int {
	step := pageStep(height) / 2
	if step < 3 {
		return 3
	}
	return step
}
