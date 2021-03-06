package calcium

import (
	"bytes"
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/projecteru2/core/store"
	"github.com/projecteru2/core/utils"

	"github.com/projecteru2/core/log"
	"github.com/projecteru2/core/types"
)

// RemoveWorkload remove workloads
// returns a channel that contains removing responses
func (c *Calcium) RemoveWorkload(ctx context.Context, ids []string, force bool, step int) (chan *types.RemoveWorkloadMessage, error) {
	logger := log.WithField("Calcium", "RemoveWorkload").WithField("ids", ids).WithField("force", force).WithField("step", step)
	ch := make(chan *types.RemoveWorkloadMessage)
	if step < 1 {
		step = 1
	}

	go func() {
		defer close(ch)
		wg := sync.WaitGroup{}
		defer wg.Wait()
		for i, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				ret := &types.RemoveWorkloadMessage{WorkloadID: id, Success: false, Hook: []*bytes.Buffer{}}
				if err := c.withWorkloadLocked(ctx, id, func(ctx context.Context, workload *types.Workload) error {
					return c.withNodeLocked(ctx, workload.Nodename, func(ctx context.Context, node *types.Node) (err error) {
						return utils.Txn(
							ctx,
							// if
							func(ctx context.Context) error {
								return errors.WithStack(c.doRemoveWorkload(ctx, workload, force))
							},
							// then
							func(ctx context.Context) error {
								log.Infof("[RemoveWorkload] Workload %s removed", workload.ID)
								return errors.WithStack(c.store.UpdateNodeResource(ctx, node, &workload.ResourceMeta, store.ActionIncr))
							},
							// rollback
							nil,
							c.config.GlobalTimeout,
						)
					})
				}); err != nil {
					logger.Errorf("[RemoveWorkload] Remove workload %s failed, err: %+v", id, err)
					ret.Hook = append(ret.Hook, bytes.NewBufferString(err.Error()))
				} else {
					ret.Success = true
				}
				ch <- ret
			}(id)
			if (i+1)%step == 0 {
				log.Info("[RemoveWorkload] Wait for previous tasks done")
				wg.Wait()
			}
		}
	}()
	return ch, nil
}

func (c *Calcium) doRemoveWorkload(ctx context.Context, workload *types.Workload, force bool) error {
	return utils.Txn(
		ctx,
		// if
		func(ctx context.Context) error {
			return errors.WithStack(workload.Remove(ctx, force))
		},
		// then
		func(ctx context.Context) error {
			return errors.WithStack(c.store.RemoveWorkload(ctx, workload))
		},
		// rollback
		nil,
		c.config.GlobalTimeout,
	)

}

// 同步地删除容器, 在某些需要等待的场合异常有用!
func (c *Calcium) doRemoveWorkloadSync(ctx context.Context, ids []string) error {
	ch, err := c.RemoveWorkload(ctx, ids, true, 1)
	if err != nil {
		return errors.WithStack(err)
	}

	for m := range ch {
		log.Debugf("[doRemoveWorkloadSync] Removed %s", m.WorkloadID)
	}
	return nil
}
