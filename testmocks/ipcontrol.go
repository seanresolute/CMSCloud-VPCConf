package testmocks

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"net"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/client"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
	"github.com/projectdiscovery/mapcidr"
)

type ContainerTree struct {
	Name       string
	ResourceID string
	Blocks     []BlockSpec
	Children   []ContainerTree
}

type MockIPControl struct {
	client.Client
	ExistingContainers ContainerTree

	ContainersAdded []ContainerSpec
	BlocksAdded     []BlockSpec
	CloudIDsUpdated map[string]string // Container name -> Cloud ID
	BlockCount      int

	ContainersDeletedWithTheirBlocks []string
	BlocksDeleted                    []string // not as part of DeleteContainer(s)WithBlocks
}

type ContainerSpec struct {
	ParentName   string
	Name         string
	BlockType    client.BlockType
	AWSAccountID string
}

type BlockSpec struct {
	ParentContainer string
	Container       string
	Address         string
	BlockType       client.BlockType
	Size            int
	Status          string
}

func (t ContainerTree) toWSContainer() *models.WSContainer {
	pieces := strings.Split(t.Name, "/")
	return &models.WSContainer{
		ParentName:    strings.Join(pieces[:len(pieces)-1], "/")[1:],
		ContainerName: pieces[len(pieces)-1],
	}
}

func toWSContainers(trees []*ContainerTree) []*models.WSContainer {
	result := []*models.WSContainer{}
	for _, t := range trees {
		result = append(result, t.toWSContainer())
	}
	return result
}

func (t *ContainerTree) filterRecursive(f func(ContainerTree) bool) []*ContainerTree {
	result := []*ContainerTree{}
	if f(*t) {
		result = append(result, t)
	}
	for i := range t.Children {
		result = append(result, (&t.Children[i]).filterRecursive(f)...)
	}
	return result
}

func (m *MockIPControl) GetContainersByName(name string) ([]*models.WSContainer, error) {
	return toWSContainers((&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.Name == name
	})), nil
}

func (m *MockIPControl) AddContainer(parentName, name string, bt client.BlockType, awsAccountID string) error {
	newContainerSpec := ContainerSpec{
		ParentName:   parentName,
		Name:         name,
		BlockType:    bt,
		AWSAccountID: awsAccountID,
	}

	matches := (&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.Name == parentName
	})
	if len(matches) != 1 {
		return fmt.Errorf("filterRecursivePointer: %d containers matching name %q", len(matches), parentName)
	}

	(matches)[0].Children = append((matches)[0].Children, ContainerTree{Name: parentName + "/" + name})

	m.ContainersAdded = append(m.ContainersAdded, newContainerSpec)
	return nil
}

func (m *MockIPControl) getUniqueContainerPointer(container string) (*ContainerTree, error) {
	matches := (&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.Name == container
	})
	if len(matches) != 1 {
		return nil, fmt.Errorf("getUniqueContainerPointer: %d containers matching name %q", len(matches), container)
	}
	return matches[0], nil
}

func (m *MockIPControl) allocateFromFree(parentContainer, container string, size int, status string, bt client.BlockType) (*models.WSChildBlock, error) {
	parent, err := m.getUniqueContainerPointer(parentContainer)
	if err != nil {
		return nil, err
	}

	targetContainer, err := m.getUniqueContainerPointer(container)
	if err != nil {
		return nil, fmt.Errorf("Unable to get target container: %v", err)
	}

	// split parent free blocks until we have a block matching the exact size we are trying to allocate (if possible)
	// Note: splitFreeBlocks also sorts tree blocks by size and then IP
	err = splitFreeBlocks(parent, size)
	if err != nil {
		return nil, err
	}

	for i := range parent.Blocks {
		block := &parent.Blocks[i]
		if block.Status == "Free" && block.Size == size {
			if status == "Aggregate" {
				//This shouldn't be a pointer to the parrent block, but its own copy
				targetContainer.Blocks = append(targetContainer.Blocks, BlockSpec{
					Size: size, Container: targetContainer.Name, Address: block.Address, BlockType: bt, Status: "Free"})
			}

			targetContainer.Blocks = append(targetContainer.Blocks, BlockSpec{
				Size: size, Container: targetContainer.Name, Address: block.Address, BlockType: bt, Status: status})

			apiBlock := &models.WSChildBlock{
				Container: parentContainer,
				BlockName: fmt.Sprintf("%s/%d", block.Address, block.Size),
			}

			if status == "Deployed" || status == "Aggregate" {
				parent.Blocks = append(parent.Blocks[:i], parent.Blocks[i+1:]...)

			} else {
				block.Status = status
			}

			return apiBlock, nil
		}
	}

	return nil, fmt.Errorf("Unable to allocate from free space")
}

func splitFreeBlocks(container *ContainerTree, targetSize int) error {
	sort.Slice(container.Blocks, func(i, j int) bool {
		if container.Blocks[i].Size == container.Blocks[j].Size {
			ipI := net.ParseIP(container.Blocks[i].Address)
			ipJ := net.ParseIP(container.Blocks[j].Address)
			return bytes.Compare(ipJ, ipI) > 0
		}
		return container.Blocks[i].Size > container.Blocks[j].Size
	})

	for i := range container.Blocks {
		if container.Blocks[i].Status != "Free" {
			continue
		}

		if container.Blocks[i].Size == targetSize {
			return nil
		}

		if container.Blocks[i].Size < targetSize {
			baseBlock := BlockSpec{Container: container.Blocks[i].Container, Status: "Free", BlockType: container.Blocks[i].BlockType}
			cidrs, err := mapcidr.SplitN(fmt.Sprintf("%s/%d", container.Blocks[i].Address, container.Blocks[i].Size), 2)
			if err != nil {
				return fmt.Errorf("Error attempting mapcidr.SplitN:")
			}
			container.Blocks = append(container.Blocks[:i], container.Blocks[i+1:]...)
			for _, cidr := range cidrs {
				splitSize, _ := cidr.Mask.Size()
				baseBlock.Address = cidr.IP.String()
				baseBlock.Size = splitSize

				container.Blocks = append(container.Blocks, baseBlock)
				err := splitFreeBlocks(container, targetSize)
				if err != nil {
					return err
				}

			}
			return nil
		}
	}
	return nil
}

func (m *MockIPControl) AllocateBlock(parentContainer, container string, bt client.BlockType, size int, status string) (*models.WSChildBlock, error) {
	hasFree, err := m.ContainerHasAvailableSpace(parentContainer, size)
	if err != nil {
		return nil, err
	}

	if hasFree {
		newAllocation, err := m.allocateFromFree(parentContainer, container, size, status, bt)
		if err != nil {
			fmt.Printf("Unable to allocate from Free Space -- parentContainer: %s container: %s size: %d, status: %s bt: %v\n", parentContainer, container, size, status, bt)
			return nil, err
		}
		return newAllocation, nil
	}

	m.BlocksAdded = append(m.BlocksAdded, BlockSpec{
		ParentContainer: parentContainer,
		Container:       container,
		BlockType:       bt,
		Size:            size,
		Status:          status,
	})
	return m.nextBlock(container, size), nil
}

func (m *MockIPControl) nextBlock(container string, size int) *models.WSChildBlock {
	block := &models.WSChildBlock{
		Container: container,
		BlockName: fmt.Sprintf("10.%d.0.0/%d", m.BlockCount, size),
	}
	m.BlockCount++
	return block
}

func (m *MockIPControl) UpdateContainerCloudID(containerName, cloudID string) error {
	targetContainer, err := m.getUniqueContainerPointer(containerName)
	if err != nil {
		return err
	}
	targetContainer.ResourceID = cloudID

	if m.CloudIDsUpdated == nil {
		m.CloudIDsUpdated = make(map[string]string)
	}
	m.CloudIDsUpdated[containerName] = cloudID
	return nil
}

func (m *MockIPControl) GetSubnetContainer(subnetID string) (*models.WSContainer, error) {
	matches := (&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.ResourceID == subnetID
	})
	if len(matches) == 0 {
		return nil, fmt.Errorf("No container for subnet %s", subnetID)
	}
	return matches[0].toWSContainer(), nil
}

func (m *MockIPControl) ListContainersForVPC(accountID, vpcID string) ([]*models.WSContainer, error) {
	matches := (&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.ResourceID == vpcID
	})
	if len(matches) == 0 {
		return nil, fmt.Errorf("No container for VPC %s", vpcID)
	}
	wsContainers := make([]*models.WSContainer, 0)
	for _, match := range matches {
		wsContainers = append(wsContainers, toWSContainers(match.filterRecursive(func(t ContainerTree) bool { return true }))...)
	}
	return wsContainers, nil
}

func (m *MockIPControl) ListBlocks(container string, onlyFree bool, recursive bool) ([]*models.WSChildBlock, error) {
	matches := (&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.Name == container
	})
	if len(matches) != 1 {
		return nil, fmt.Errorf("ListBlocks: %d containers matching name %q", len(matches), container)
	}
	if recursive {
		matches = matches[0].filterRecursive(func(t ContainerTree) bool { return true })
	}
	blocks := []*models.WSChildBlock{}
	for _, c := range matches {
		for _, block := range c.Blocks {
			if onlyFree != (block.Status == "free") {
				continue
			}
			blocks = append(blocks, &models.WSChildBlock{
				Container: c.Name,
				BlockAddr: block.Address,
				BlockSize: fmt.Sprintf("%d", block.Size),
			})
		}
	}
	return blocks, nil
}

func (m *MockIPControl) ContainerHasAvailableSpace(container string, size int) (bool, error) {
	matches := (&m.ExistingContainers).filterRecursive(func(t ContainerTree) bool {
		return t.Name == container
	})
	if len(matches) != 1 {
		return false, fmt.Errorf("ContainerHasAvailableSpace: %d containers matching name %q", len(matches), container)
	}
	for _, c := range matches {
		for _, block := range c.Blocks {
			if block.Status == "Free" && block.Size <= size {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *MockIPControl) DeleteContainersAndBlocks(containerNames []string, logger client.Logger) error {
	sort.Strings(containerNames)
	m.ContainersDeletedWithTheirBlocks = append(m.ContainersDeletedWithTheirBlocks, containerNames...)
	return nil
}

func (m *MockIPControl) DeleteBlock(name, container string, logger client.Logger) error {
	m.BlocksDeleted = append(m.BlocksDeleted, name)
	return nil
}
