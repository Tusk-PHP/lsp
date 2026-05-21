<?php

namespace App\Controller;

use Symfony\Bundle\FrameworkBundle\Controller\AbstractController;
use Symfony\Component\HttpFoundation\JsonResponse;
use Symfony\Component\HttpFoundation\Request;
use Symfony\Component\HttpFoundation\Response;
use Symfony\Component\Routing\Attribute\Route;
use App\Repository\ProductRepository;
use App\Service\NotificationService;
use App\Entity\Product;

#[Route('/products', name: 'product_')]
class ProductController extends AbstractController
{
    public function __construct(
        private ProductRepository $repo,
        private NotificationService $notifier
    ) {}

    #[Route('', name: 'index')]
    public function index(): JsonResponse
    {
        // Injected service method with return type
        $allProducts = $this->repo->findAll();

        // Typed return from repository
        $first = $this->repo->find(1);

        // Response creation
        $response = new JsonResponse(['products' => $allProducts]);
        $listingUrl = $this->generateUrl('product_index');

        return $response;
    }

    #[Route('/{id}', name: 'show')]
    public function show(Request $request): Response
    {
        // Request parameter access
        $id = $request->get('id');

        // Method on injected service
        $this->notifier->notify('Product viewed');

        // Entity construction and method chain
        $product = new Product();
        $product->setName('Test');
        $productName = $product->getName();

        // Typed return from repository
        $found = $this->repo->find(1);

        // Array shape
        $data = ['id' => 1, 'name' => 'Test Product', 'price' => 9.99];
        $detailsUrl = $this->generateUrl('product_show');

        return new Response('OK');
    }
}
