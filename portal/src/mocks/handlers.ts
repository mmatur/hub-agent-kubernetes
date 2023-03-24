/*
Copyright (C) 2022-2023 Traefik Labs
This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.
You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

import { rest } from 'msw'
import api from './api.json'
import collectionApi from './collection-api.json'

export const handlers = [
  rest.post('/login', (req, res, ctx) => {
    return res(
      // TODO confirm how the response going to be, this is fully assumed
      ctx.status(200),
      ctx.json({
        accessToken: 'mockt0ken18683jbdb',
        user: {
          username: 'user@email.com',
        },
      }),
    )

    /* To mock fail request*/

    // return res(
    //   ctx.status(401),
    //   ctx.json({
    //     errorMessage: 'Unauthorized',
    //   }),
    // )
  }),

  rest.get('/api/:portalName/apis', (req, res, ctx) => {
    // const headers = req.headers
    // if (headers.get('Authorization')) {
    return res(
      ctx.status(200),
      ctx.json({
        collections: [
          {
            name: 'my-empty-store-collection',
            apis: [],
          },
          {
            name: 'my-store-collection',
            pathPrefix: '/api',
            apis: [
              {
                name: 'my-petstore-api',
                specLink: '/collections/my-store-collection/apis/my-petstore-api@petstore',
                pathPrefix: '/prefix',
              },
            ],
          },
          {
            name: 'my-store-collection-2',
            apis: [
              {
                name: 'my-petstore-api-2',
                specLink: '/collections/my-store-collection/apis/my-petstore-api@petstore',
                pathPrefix: '/path',
              },
            ],
          },
        ],
        apis: [{ name: 'my-petstore-api', specLink: '/apis/my-petstore-api@petstore', pathPrefix: '/api' }],
      }),
    )
    // } else {
    //   return res(
    //     ctx.status(401),
    //     ctx.json({
    //       errorMessage: 'Unauthorized',
    //     }),
    //   )
    // }
  }),

  rest.get('/api/:portalName/apis/:apiName', (req, res, ctx) => {
    // const headers = req.headers
    // if (headers.get('Authorization')) {
    return res(ctx.status(200), ctx.json(api))
    // } else {
    //   return res(
    //     ctx.status(401),
    //     ctx.json({
    //       errorMessage: 'Unauthorized',
    //     }),
    //   )
    // }
  }),

  rest.get('/api/:portalName/collections/:collectionName/apis/:apiName', (req, res, ctx) => {
    // const headers = req.headers
    // if (headers.get('Authorization')) {
    return res(ctx.status(200), ctx.json(collectionApi))
    // } else {
    //   return res(
    //     ctx.status(401),
    //     ctx.json({
    //       errorMessage: 'Unauthorized',
    //     }),
    //   )
    // }
  }),
]
