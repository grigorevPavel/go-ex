package orderbook

import "errors"

func NewExpiryHeap() *ExpiryHeap {
	return &ExpiryHeap{
		nodes:   make([]*ExpiryHeapNode, 0),
		indices: make(map[OrderID]int),
	}
}

func (h *ExpiryHeap) Top() (*ExpiryHeapNode, error) {
	if len(h.nodes) == 0 {
		return nil, errors.New("heap is empty")
	}
	return h.nodes[0], nil
}

func (h *ExpiryHeap) Pop() (*ExpiryHeapNode, error) {
	if len(h.nodes) == 0 {
		return nil, errors.New("heap is empty")
	}
	
	// copy the top element
	top := h.nodes[0]

	// swap top with the last element
	h.swapNodesByIndex(0, len(h.nodes)-1)

	// remove the last element & free orderID map
	h.nodes = h.nodes[:len(h.nodes)-1]
	delete(h.indices, top.OrderID)

	// sift down the new top element
	if len(h.nodes) > 1 {
		h.siftDown(0)
	}

	return top, nil
}

func (h *ExpiryHeap) Push(node *ExpiryHeapNode) {
	// add element to the end of the array & set the orderID map
	i := len(h.nodes)
	h.nodes = append(h.nodes, node)
	h.indices[node.OrderID] = i

	// sift up the new element
	if len(h.nodes) > 1 {
		h.siftUp(i)
	}
}

func (h *ExpiryHeap) Remove(orderID OrderID) (*ExpiryHeapNode, error) {
	i, ok := h.indices[orderID]
	if !ok {
		return nil, errors.New("order not found")
	}

	// copy the node to be removed
	res := h.nodes[i]

	// swap the node with the last element
	h.swapNodesByIndex(i, len(h.nodes)-1)

	// remove the last element & free orderID map
	h.nodes = h.nodes[:len(h.nodes)-1]
	delete(h.indices, orderID)

	if len(h.nodes) > 1 && i < len(h.nodes) {
		h.siftUp(i)
		h.siftDown(i)
	}

	return res, nil
}

func parent(i int) int {
	return (i - 1) / 2 // floor division
}

func leftChild(i int) int {
	return 2*i + 1
}

func rightChild(i int) int {
	return 2*i + 2
}

func (h *ExpiryHeap) siftUp(i int) {
	for i > 0 {
		parentIndex := parent(i)
		if !h.cmpLess(parentIndex, i) {
			h.swapNodesByIndex(i, parentIndex)

			// update the index
			i = parentIndex
		} else {
			break
		}
	}
}

func (h *ExpiryHeap) siftDown(i int) {
	for {
		leftChildIndex := leftChild(i)
		rightChildIndex := rightChild(i)
		smallest := i

		// first compare with the left child
		// left child if farter away from the root than the right child
		if leftChildIndex < len(h.nodes) && h.cmpLess(leftChildIndex, smallest) {
			smallest = leftChildIndex
		}

		// then compare with the right child
		if rightChildIndex < len(h.nodes) && h.cmpLess(rightChildIndex, smallest) {
			smallest = rightChildIndex
		}

		// break if tree invariant is fulfilled (or tree is over)
		if smallest == i {
			break
		}

		h.swapNodesByIndex(i, smallest)

		// update the index
		i = smallest
	}
}	

func (h *ExpiryHeap) swapNodesByIndex(i int, j int) {
	// skip the trivial case
	if i == j {
		return
	}

	// swap the nodes
	tmp := h.nodes[i]
	h.nodes[i] = h.nodes[j]
	h.nodes[j] = tmp

	// swap the indices
	h.indices[tmp.OrderID] = j
	h.indices[h.nodes[i].OrderID] = i
}

func (h *ExpiryHeap) cmpLess(i int, j int) bool {
	a, b := h.nodes[i], h.nodes[j]
	if a.Expiry < b.Expiry {
		return true
	} else if a.Expiry > b.Expiry {
		return false
	}

	// guarantees the strict total order (determined by the orderID)
	return a.OrderID < b.OrderID
}
