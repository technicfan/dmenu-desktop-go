package main

type LinkedListNode[T any] struct {
	Value T
	next *LinkedListNode[T]
	previous *LinkedListNode[T]
}

type LinkedList[T any] struct {
	head *LinkedListNode[T]
}

func NewLinkedList[T any]() *LinkedList[T] {
	var null T
	return &LinkedList[T]{&LinkedListNode[T]{null, nil, nil}}
}

func (head *LinkedList[T]) InsertSorted(value T, cmp func(a, b T) int) *LinkedListNode[T] {
	new_node := &LinkedListNode[T]{value, nil, nil}
	previous := head.head
	current := previous.next
	for current != nil {
		if cmp(value, current.Value) < 0 {
			new_node.previous = current.previous
			new_node.previous.next = new_node
			new_node.next = current
			current.previous = new_node
			break
		}
		previous = current
		current = current.next
	}
	if current == nil {
		new_node.previous = previous
		previous.next = new_node
	}
	return new_node
}

func (node *LinkedListNode[T]) Remove() {
	if node.previous == nil {
		return
	}
	node.previous.next = node.next
	if node.next != nil {
		node.next.previous = node.previous
	}
}

func (head *LinkedList[T]) ForEach(action func(i int, value T)) {
	current := head.head.next
	for i := 0; current != nil; i++ {
		action(i, current.Value)
		current = current.next
	}
}
